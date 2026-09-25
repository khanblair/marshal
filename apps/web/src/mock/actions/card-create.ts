import { defaultAgent, defaultModel } from "../agents";
import { nextCardNumber } from "../card-key";
import type { Ctx } from "../context";
import { live, pushMsg, set, toast } from "../engine";
import { type CardSeed, makeCard } from "../seed/cards";
import { checksFor } from "../seed/checks";
import type { Activity, Card, Column, Msg, NewCardDraft } from "../types";
import { openCard } from "./navigation";
import { startSession } from "./sessions";

/** New cards are planned for today through the next three days. */
const NEW_CARD_SPAN = { s: 0, e: 3 };
const FALLBACK_MODEL = "claude-sonnet-4-5";

/**
 * Creates a card with the next number in its project. Like the prototype, `upd: 0` means "at load
 * time", not the current time.
 */
export function newCardFrom(ctx: Ctx, seed: Omit<CardSeed, "n">): Card {
  return makeCard(ctx.loadedAt, { n: nextCardNumber(ctx.S.cards, seed.p), ...seed });
}

/** Adds a card to the end of `S.cards` with its chat, activity, and checks. Returns the live card. */
export function insertCard(ctx: Ctx, c: Card, chat: Msg[] = [], act: Activity[] = []): Card {
  const { S } = ctx;
  const item = live(c);
  S.cards.push(item);
  S.chat[c.id] = chat;
  S.act[c.id] = act;
  S.checks[c.id] = checksFor(c);
  return item;
}

/** The board's inline "Add card". Planning and working columns start the session right away. */
export function quickAdd(ctx: Ctx, pid: string, col: Column, title: string): void {
  if (!title?.trim()) return;
  const c = insertCard(
    ctx,
    newCardFrom(ctx, {
      p: pid,
      title: title.trim(),
      state: "backlog",
      upd: 0,
      ...NEW_CARD_SPAN,
      perm: col === "planning" ? "Plan only" : "Auto-accept edits",
      members: ["ada"],
    }),
  );
  if (col === "planning" || col === "working") startSession(ctx, c, col);
  toast(ctx, col === "backlog" ? "Card created" : "Card created and started", {
    label: "Open",
    run: () => openCard(ctx, c.id),
  });
}

export function newCard(ctx: Ctx, opts?: Partial<NewCardDraft>): void {
  set(ctx, {
    newCard: {
      title: "",
      body: "",
      template: "Blank",
      role: "Worker",
      agent: defaultAgent(ctx),
      start: false,
      ...opts,
    },
  });
}

/** Creates the card drafted in the New card dialog, in the current project. */
export function createCard(ctx: Ctx): void {
  const { S } = ctx;
  const n = S.newCard;
  const pid = S.route.pid;
  if (!n?.title.trim() || !pid) return;
  const c = insertCard(
    ctx,
    newCardFrom(ctx, {
      p: pid,
      title: n.title.trim(),
      state: "backlog",
      role: n.role,
      agent: n.agent,
      model: defaultModel(ctx, n.agent) ?? FALLBACK_MODEL,
      upd: 0,
      ...NEW_CARD_SPAN,
      perm: n.template === "Plan first" ? "Plan only" : "Auto-accept edits",
    }),
  );
  if (n.body) pushMsg(ctx, c.id, ctx.msg.user(n.body));
  S.newCard = null;
  if (n.start) startSession(ctx, c);
  toast(ctx, "Card created", { label: "Open", run: () => openCard(ctx, c.id) });
}
