import type { CreateCardRequest, UpdateCardRequest } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { ApiError } from "~/data/api-error";
import { agentKindOf, permissionModeOf, thinkingModeOf } from "~/data/mappers/card";
import { ChangeInFlightError } from "~/data/optimistic";
import { openCard } from "~/mock/actions/navigation";
import { defaultModel, thinkSupported } from "~/mock/agents";
import type { CardKey } from "~/mock/card-key";
import { colOf } from "~/mock/constants";
import type { Ctx } from "~/mock/context";
import { confirm, later, toast } from "~/mock/engine";
import { cardLabelOf, card as cardOf } from "~/mock/selectors";
import type { Card, Column } from "~/mock/types";
import { applyCard, applyCardRemoved } from "./cards";

/*
 * The write path of the cards: section S5a (the board's move, rename, settings, delete, fork,
 * create, and start) and S7a (the card header's start and stop). Each function is the daemon-backed
 * version of one the screens call on the mock today, and keeps that one's arguments.
 *
 * The daemon owns every card field, so a success is put into the store through the same functions
 * the `card.*` events use (`applyCard`, `applyCardRemoved`); the event the daemon publishes after
 * the change then finds the card as it already is and draws nothing. A move is the one exception:
 * its drag has to land somewhere before the daemon answers, so the card takes the new column at
 * once and is put back when the daemon refuses, exactly as the mock's own move did.
 *
 * The daemon also owns the rules of a move. Nothing here decides whether a card may go to a column:
 * a refusal carries the daemon's reason code and its plain sentence, and that sentence is what the
 * caller and the toast show.
 */

/** The model a card falls back to when its agent is one Marshal does not know. */
const FALLBACK_MODEL = "claude-sonnet-4-5";
const NOT_CONNECTED = "Marshal is not connected to its daemon.";
const GENERIC_FAILURE = "Marshal could not finish that. Try again.";
/** A refused card sits in the column it was dragged to this long before it snaps back, as on the mock. */
const SNAP_BACK_MS = 420;

function apiOf(ctx: Ctx): ApiClient | null {
  return ctx.env.data?.api ?? null;
}

/** The daemon's own sentence when it wrote one, and the same plain sentence the project actions use otherwise. */
const sentence = (error: unknown): string =>
  error instanceof ApiError ? error.message : GENERIC_FAILURE;

/** A change draws nothing before the daemon answers, so there is nothing to put back if it refuses. */
const nothing = (): void => undefined;

/**
 * The id the daemon's card routes take. The store is keyed by the card's key, which is what every
 * screen and map uses, and the two are not the same: a route given a key answers not_found. A card
 * the mock made has no daemon id, and nothing here can act on it.
 */
const daemonIdOf = (card: Card): string => card.daemonId ?? "";

/**
 * Asks the daemon for one change and hands its answer back, or null when nothing was asked or the
 * daemon said no. `optimistic` is what shows a refusal with the daemon's own plain sentence, and
 * what refuses a second change to the same card while one is still being saved.
 */
async function ask<T>(
  ctx: Ctx,
  key: string,
  run: (api: ApiClient) => Promise<T>,
): Promise<T | null> {
  const api = apiOf(ctx);
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return null;
  }
  try {
    return await ctx.optimistic({
      key,
      apply: nothing,
      request: () => run(api),
      rollback: nothing,
    });
  } catch (error) {
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
    return null;
  }
}

/** The same, for a change whose answer is not needed: true when the daemon accepted it. */
async function attempt(
  ctx: Ctx,
  key: string,
  run: (api: ApiClient) => Promise<unknown>,
): Promise<boolean> {
  return (await ask(ctx, key, run)) !== null;
}

/**
 * Moves a card by hand, the way the mock's own move did: the card takes the new column at once, so
 * a drag lands, then the daemon says whether it may stay. A refused move is put back after the
 * mock's own pause, with the daemon's sentence, which is also handed back so the drag that made it
 * can snap back with the same words. The daemon's rules are never copied here.
 */
export async function moveCard(ctx: Ctx, id: CardKey, to: Column): Promise<string | null> {
  const card = cardOf(ctx, id);
  if (!card || colOf(card.state) === to) return null;
  const api = apiOf(ctx);
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return NOT_CONNECTED;
  }
  const from = card.state;
  card.state = to;
  try {
    applyCard(ctx, await api.moveCard(daemonIdOf(card), { state: to }));
    return null;
  } catch (error) {
    const why = sentence(error);
    later(SNAP_BACK_MS, () => {
      const moved = cardOf(ctx, id);
      if (moved) moved.state = from;
      toast(ctx, why);
    });
    return why;
  }
}

/**
 * The names the card panel uses for the settings a person changes. The mock's own `SettingKey`,
 * kept here because this module replaces the mock action that used it.
 */
type SettingKey = "agent" | "role" | "model" | "think" | "perm";

/**
 * The prototype's own rule for the thinking setting when a card's model changes: a model that cannot
 * be told how hard to think carries none, and one that can carries Medium when it had nothing. The
 * daemon changes only the fields it is sent, so the panel's rule travels with the change - the
 * mock's `fixThinking` did exactly this.
 */
function thinkingFor(ctx: Ctx, card: Card, model: string): UpdateCardRequest {
  if (!thinkSupported(ctx, model)) return { thinking: "" };
  return card.think ? {} : { thinking: thinkingModeOf("Medium") };
}

/**
 * The fields of the edit request one setting means, or null for a name Marshal does not know. An
 * agent carries the model that agent starts its cards on, the way the mock's own setting did: the
 * daemon would otherwise leave the card holding another agent's model.
 */
function changeOf(ctx: Ctx, card: Card, key: SettingKey, value: string): UpdateCardRequest | null {
  if (key === "agent") {
    const agent = agentKindOf(value);
    if (!agent) return null;
    const model = defaultModel(ctx, value) ?? card.model;
    return { agent, model, ...thinkingFor(ctx, card, model) };
  }
  if (key === "think") {
    const thinking = thinkingModeOf(value);
    return thinking ? { thinking } : null;
  }
  if (key === "perm") {
    const permissionMode = permissionModeOf(value);
    return permissionMode ? { permissionMode } : null;
  }
  if (key === "role") return { role: value };
  return { model: value, ...thinkingFor(ctx, card, value) };
}

/**
 * Changes one agent setting. The panel's names become the wire's through the card mapper, and only
 * the field that changed is sent, so a save cannot undo another device's edit of another field. A
 * name Marshal does not know asks the daemon for nothing.
 */
export async function setSetting(
  ctx: Ctx,
  id: CardKey,
  key: SettingKey,
  value: string,
): Promise<boolean> {
  const card = cardOf(ctx, id);
  if (!card) return false;
  const body = changeOf(ctx, card, key, value);
  if (!body) return false;
  return attempt(ctx, `update:${id}`, async (api) => {
    applyCard(ctx, await api.updateCard(daemonIdOf(card), body));
  });
}

/** Renames a card. An empty name keeps the old one without asking, the way the mock's rename did. */
export async function rename(ctx: Ctx, id: CardKey, title: string): Promise<boolean> {
  const card = cardOf(ctx, id);
  const next = title.trim();
  if (!card || !next || next === card.title) return false;
  return attempt(ctx, `update:${id}`, async (api) => {
    applyCard(ctx, await api.updateCard(daemonIdOf(card), { title: next }));
  });
}

/**
 * Removes a card on the daemon, after asking. The mock's own delete put the question up itself and
 * its menu item still relies on that, so the same confirmation is shown here, with the same words,
 * and the daemon is asked only once the person says yes. A card the store no longer has is not
 * asked about, so a second removal of the same card is harmless.
 */
export async function deleteCard(ctx: Ctx, id: CardKey): Promise<boolean> {
  const card = cardOf(ctx, id);
  if (!card) return false;
  confirm(ctx, {
    title: "Delete card",
    message: card.branch
      ? `This deletes ${cardLabelOf(ctx, card)}, its session, and its worktree with unmerged work on ${card.branch}.`
      : `This deletes ${cardLabelOf(ctx, card)} and its notes.`,
    action: "Delete card",
    destructive: true,
    run: () => void removeOnDaemon(ctx, id, daemonIdOf(card)),
  });
  return true;
}

/** The removal itself, run from the confirmation above: the daemon first, then the store. */
async function removeOnDaemon(ctx: Ctx, id: CardKey, daemonId: string): Promise<boolean> {
  return attempt(ctx, `remove:${id}`, async (api) => {
    await api.removeCard(daemonId);
    applyCardRemoved(ctx, id);
    toast(ctx, "Card deleted");
  });
}

/**
 * Forks a card. The daemon numbers the copy and names it, so nothing about it is guessed here; it
 * goes in through `applyCard`, the same function its `card.created` event uses.
 */
export async function fork(ctx: Ctx, id: CardKey): Promise<boolean> {
  const card = cardOf(ctx, id);
  if (!card) return false;
  const forked = await ask(ctx, `fork:${id}`, async (api) => {
    const created = await api.forkCard(daemonIdOf(card));
    applyCard(ctx, created);
    toast(ctx, "Card forked", { label: "Open", run: () => openCard(ctx, created.key) });
    return created.key;
  });
  return forked !== null;
}

/** What a board lane adds to a card created inside it: the fields the lanes group cards by. */
interface LaneExtra {
  role?: string;
  agent?: string;
  pkg?: string | null;
  model?: string;
}

/** Adds a card on the daemon and says so, with the same Open shortcut the mock's "Add card" had. */
async function addCard(
  ctx: Ctx,
  pid: string,
  request: CreateCardRequest,
  done: string,
): Promise<CardKey | null> {
  return ask(ctx, `create:${pid}`, async (api) => {
    const created = await api.createCard(pid, request);
    applyCard(ctx, created);
    toast(ctx, done, { label: "Open", run: () => openCard(ctx, created.key) });
    return created.key;
  });
}

/**
 * The board's inline "Add card". The column is where the card is added and, for Planning and
 * Working, what makes the daemon start its session: adding a card in a started column is one
 * request, so nothing is ever shown working before its agent exists.
 */
export async function quickAdd(
  ctx: Ctx,
  pid: string,
  col: Column,
  title: string,
  extra: LaneExtra = {},
): Promise<CardKey | null> {
  if (!title.trim()) return null;
  const planning = col === "planning";
  return addCard(
    ctx,
    pid,
    {
      title: title.trim(),
      // An empty model means the agent's own default, which is what a lane without one wants.
      model: extra.model,
      role: extra.role,
      agent: extra.agent ? agentKindOf(extra.agent) : undefined,
      package: extra.pkg ?? undefined,
      permissionMode: planning ? "plan" : "auto-edits",
      startState: col,
    },
    col === "backlog" ? "Card created" : "Card created and started",
  );
}

/**
 * Creates the card drafted in the New card dialog, in the project the app is on. The dialog's
 * template is the app's own idea -- "Plan first" means the card starts in plan-only mode -- so it
 * becomes the permission mode and, when the draft says to start at once, the column the card is
 * added in. The draft stays until the daemon accepts it, so a refusal leaves the dialog open.
 */
export async function createCard(ctx: Ctx): Promise<CardKey | null> {
  const draft = ctx.S.newCard;
  const pid = ctx.S.route.pid;
  if (!draft?.title.trim() || !pid) return null;
  const planning = draft.template === "Plan first";
  const created = await addCard(
    ctx,
    pid,
    {
      title: draft.title.trim(),
      body: draft.body,
      role: draft.role,
      agent: agentKindOf(draft.agent),
      // The agent's own default model, which is what the mock's dialog wrote on the card. The
      // daemon reads an empty model as "the agent's own default" when it starts the session, so
      // naming it here keeps the two paths alike on the card a person sees.
      model: defaultModel(ctx, draft.agent) ?? FALLBACK_MODEL,
      permissionMode: planning ? "plan" : "auto-edits",
      startState: draft.start ? (planning ? "planning" : "working") : "backlog",
    },
    "Card created",
  );
  if (created !== null) ctx.S.newCard = null;
  return created;
}

/**
 * Starts a card's session. The daemon answers the card as it now is, which `applyCard` puts in.
 * Starting a paused card is how a pause is released (the card panel's Resume card, section S7c), and
 * the mock says "Card resumed" for that, so the toast does too.
 */
export async function start(ctx: Ctx, id: CardKey): Promise<boolean> {
  const card = cardOf(ctx, id);
  if (!card) return false;
  const resuming = card.paused;
  return attempt(ctx, `session:${id}`, async (api) => {
    applyCard(ctx, await api.startCard(daemonIdOf(card)));
    toast(ctx, resuming ? "Card resumed" : "Card started");
  });
}

/**
 * Stops a card's session. The daemon answers with no body, so the card is left to the
 * `card.updated` event that follows; the mock has no line for this yet, so none is said here.
 */
export async function stop(ctx: Ctx, id: CardKey): Promise<boolean> {
  const card = cardOf(ctx, id);
  if (!card) return false;
  return attempt(ctx, `session:${id}`, (api) => api.stopCard(daemonIdOf(card)));
}
