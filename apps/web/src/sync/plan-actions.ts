import { type ChatPlan, EventTypePlanUpdated, type Event as WireEvent } from "@marshal/protocol";
import { batch } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import { ChangeInFlightError } from "~/data/optimistic";
import { isDaemon } from "~/data/sections";
import type { CardKey } from "~/mock/card-key";
import { type Ctx, sectionsOf } from "~/mock/context";
import { toast } from "~/mock/engine";
import { card as cardOf } from "~/mock/selectors";
import { applyCard } from "./cards";
import { planBlock } from "./chat-mapper";

/*
 * The plan's own writes and its one event (section S8c): the answers a person gives to the plan a
 * card is waiting on, and the plan those answers leave behind.
 *
 * A plan lives in the card's chat as a message and not in a table of its own
 * (docs/marshal-product-scope.md 10.3), and the daemon answers with the card the answer moved. So
 * an answer here is one route call and `applyCard`, the same function the `card.*` events use, and
 * the `plan.updated` that follows then finds the card as it already is and draws nothing.
 *
 * The daemon owns where a plan stands: nothing here decides whether an answer is allowed, and a
 * refusal carries its own plain sentence, which `optimistic` shows.
 */

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

/**
 * True while the plans are the daemon's. The mock's own writes stay where they are, in the plan
 * block's own view (`mock/deco-msgs.ts`), which picks this or that by `plansOnDaemon`, the way the
 * Roles screen picks by `rolesOnDaemon` (views/settings/role-actions.ts).
 */
export function plansOnDaemon(ctx: Ctx): boolean {
  return isDaemon("S8c", sectionsOf(ctx.env));
}

/**
 * The id the daemon's card routes take. The store is keyed by the card's key, and the two are not
 * the same thing: a card the mock made has no daemon id, and nothing here can act on it.
 */
const daemonIdOf = (card: { daemonId?: string | undefined }): string => card.daemonId ?? "";

/** A change draws nothing before the daemon answers, so there is nothing to put back if it refuses. */
const nothing = (): void => undefined;

/**
 * Answers the plan with one route call and puts the card it answers for into the store. False means
 * nothing was asked, or the daemon said no; either way the reason has already been shown.
 *
 * The change is named after the plan it answers, so two answers to the same card cannot overlap; the
 * card's own key is kept apart from that name, because one is what the store is keyed by and the
 * other is what the daemon's routes take.
 */
async function answer(ctx: Ctx, key: CardKey, run: (api: ApiClient, id: string) => Promise<void>) {
  const card = cardOf(ctx, key);
  const id = card ? daemonIdOf(card) : "";
  if (!card || !id) return false;
  const api = ctx.env.data?.api;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return false;
  }
  try {
    await ctx.optimistic({
      key: `plan:${key}`,
      apply: nothing,
      request: () => run(api, id),
      rollback: nothing,
    });
    return true;
  } catch (error) {
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
    return false;
  }
}

/** Starts work on the card's plan: it stops waiting, the card is working, and it leaves plan-only. */
export async function approvePlan(ctx: Ctx, key: CardKey): Promise<boolean> {
  const approved = await answer(ctx, key, async (api, id) => {
    applyCard(ctx, await api.approvePlan(id));
  });
  if (approved) toast(ctx, "Plan approved");
  return approved;
}

/** Sends the plan back for another one: the card returns to planning. */
export async function rejectPlan(ctx: Ctx, key: CardKey): Promise<boolean> {
  const rejected = await answer(ctx, key, async (api, id) => {
    applyCard(ctx, await api.rejectPlan(id));
  });
  if (rejected) toast(ctx, "Plan rejected");
  return rejected;
}

/**
 * Saves the steps a person left in the plan's editor, one per line. The daemon drops the blank
 * lines and refuses a plan with no steps left, with its own sentence, so nothing is trimmed or
 * judged here. Only the steps change: the files, the risks, and the checks are the agent's own
 * reading of the work. The plan stays waiting, and stays the plan a person answers.
 */
export async function savePlan(ctx: Ctx, key: CardKey, text: string): Promise<boolean> {
  const steps = text.split("\n");
  const saved = await answer(ctx, key, async (api, id) => {
    applyCard(ctx, await api.editPlan(id, { steps }));
  });
  if (saved) toast(ctx, "Plan saved");
  return saved;
}

/** True when a value is a plan block, so a malformed event is ignored rather than drawn. */
function isChatPlan(value: unknown): value is ChatPlan {
  if (!isRecord(value) || typeof value.state !== "string") return false;
  return (["steps", "files", "risks", "checks"] as const).every((field) =>
    Array.isArray(value[field]),
  );
}

/** The newest plan block in a chat, which is the one a card is waiting on. */
function newestPlanAt(chat: readonly { k: string }[]): number {
  for (let at = chat.length - 1; at >= 0; at -= 1) {
    if (chat[at]?.k === "plan") return at;
  }
  return -1;
}

/**
 * Applies one `plan.updated` to the open card's chat, so an answer given in one view of a card is
 * the answer every view of it draws.
 *
 * The daemon replaces a plan rather than editing it in place, so an answer is a newer plan message
 * with an id of its own (internal/history/plan.go, AppendPlan). A chat draws the newest plan it
 * holds and drops the ones before it, so this takes the place of the newest plan the store has
 * instead of adding a second one: the screen then draws what a read of the chat would page back.
 */
export function applyPlanUpdated(ctx: Ctx, event: WireEvent): void {
  if (event.type !== EventTypePlanUpdated) return;
  if (!plansOnDaemon(ctx)) return;
  const data = isRecord(event.data) ? event.data : null;
  if (!data || typeof data.cardId !== "string" || typeof data.messageId !== "string") return;
  if (!isChatPlan(data.plan)) return;
  const key = ctx.S.cards.find((one) => one.daemonId === data.cardId)?.id;
  if (!key) return;
  const chat = ctx.S.chat[key] ?? [];
  ctx.S.chat[key] = chat;
  const block = planBlock(data.messageId, data.plan);
  const at = newestPlanAt(chat);
  batch(() => {
    if (at === -1) chat.push(block);
    else chat[at] = block;
  });
}
