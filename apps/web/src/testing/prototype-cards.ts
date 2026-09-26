/**
 * The prototype's 29 cards, as the daemon would send them.
 *
 * Section S5a moves the cards to the daemon, and the reservoir then keeps the mock's own 29 out of
 * the store for good (`reconcileMock`). The unit tests still draw a board, a List, a Timeline, and
 * the Agents view, so they need the cards the prototype always showed. This gives them to the test
 * store the way the app will get them: the prototype's seed data becomes wire cards, the real
 * mirror (`applyCardSnapshot`) fills the store, and then the fields the daemon sends no column for
 * are put back. It is the test store's copy of the daemon's own fixture (`daemon/internal/fixture`),
 * and only tests use it.
 */
import type {
  Label,
  LabelColor,
  NeedsReason,
  NeedsReasonKind,
  SessionState,
  Card as WireCard,
} from "@marshal/protocol";
import { agentKindOf, permissionModeOf, thinkingModeOf } from "~/data/mappers/card";
import type { Ctx } from "~/mock/context";
import { createIds } from "~/mock/ids";
import { seedCardExtras } from "~/mock/seed/card-extras";
import { seedCards } from "~/mock/seed/cards";
import type { Card } from "~/mock/types";
import { applyCardSnapshot, type DaemonBoard } from "~/sync/cards";
import { boardOf, wireCard } from "./fake-cards";

/** A day in milliseconds, the unit the store's `s`, `e`, and `due` count in. */
const DAY_MS = 86_400_000;

/** The fraction a card's `ctx` is, against the percentage from 0 to 100 that the daemon sends. */
const PERCENT = 100;

/**
 * The colour of every prototype label. The prototype names its labels and never colours them, and
 * the mapper reads only `label.name`, so one of the wire's fixed colours is named and no more.
 */
const LABEL_COLOR: LabelColor = "slate";

/**
 * Why the four prototype cards that wait on a person wait, from the same data as the daemon's own
 * fixture. The prototype kept only the sentence a person reads, so the kind is the daemon's; a card
 * that has a reason and no kind of its own is the general question.
 */
const REASON_KINDS: Readonly<Record<string, NeedsReasonKind>> = {
  "api#43": "plan-ready",
  "api#44": "approval-needed",
  "web#119": "stuck",
  "mobile#210": "conflict",
};

/**
 * The card fields the daemon sends no column for. The mirror writes its own empty values for them,
 * and the prototype's own are put back over the top (its costs, dependencies, members, checklists,
 * comments, merge percent, and its bypass flag). Asleep and waking are not among them: the wire card
 * carries a session, and the mirror draws both flags from it.
 */
const PROTOTYPE_ONLY = [
  "cost",
  "deps",
  "members",
  "checklists",
  "comments",
  "mergePct",
  "bypass",
] as const;

/** Midnight of the day a moment falls on, which is where `dayOf` counts a card's day numbers from. */
const startOfDay = (at: number): number => {
  const day = new Date(at);
  day.setHours(0, 0, 0, 0);
  return day.getTime();
};

/** The moment a day number means, so that the mirror's `dayOf` gives the same number back. */
const atDay = (base: number, day: number | null): string | null =>
  day === null ? null : new Date(base + day * DAY_MS).toISOString();

/** A name the mapper knows, or a loud failure: the prototype's cards only name settings it knows. */
const mustKnow = <T>(value: T | undefined, what: string): T => {
  if (value === undefined) throw new Error(`the prototype's cards name an unknown ${what}`);
  return value;
};

/** The prototype's label names as the wire's labels. Only the name survives the mirror. */
const wireLabels = (card: Card): Label[] =>
  card.labels.map((name) => ({
    id: `${card.id}:${name}`,
    projectId: card.p,
    name,
    color: LABEL_COLOR,
    createdAt: new Date(card.upd).toISOString(),
  }));

/** Why a card waits, as the daemon sends it: the prototype's own sentence, and the daemon's kind. */
const wireReason = (card: Card): NeedsReason | null =>
  card.reason === "" ? null : { kind: REASON_KINDS[card.id] ?? "question", text: card.reason };

/**
 * The session a prototype card has, as the daemon's own fixture seeds it (`fixtureSessionState`): a
 * card on a turn is working, one that has started and waits is awake, and a card in the backlog or
 * done has none. The prototype's sleeping cards read asleep, which the fixture does not seed, so the
 * cards the mock drew asleep are drawn asleep here too and the two paths stay the same.
 */
function sessionOf(card: Card): SessionState | null {
  if (card.waking) return "waking";
  if (card.asleep) return "asleep";
  if (card.state === "working" || card.state === "planning" || card.state === "merging") {
    return "working";
  }
  if (card.state === "needs" || card.state === "review" || card.state === "ready") return "awake";
  return null;
}

/**
 * One prototype card as the daemon would send it. Every field the mapper reads is set from the
 * card, because the golden card the helper starts from carries none of the prototype's own data.
 */
const wireFrom = (card: Card, base: number): WireCard =>
  wireCard({
    projectId: card.p,
    number: card.n,
    title: card.title,
    // Nothing reads a card's body or its creation time from the store, and the prototype has
    // neither, so the body is empty and the card's last change stands in for both moments.
    body: "",
    state: card.state,
    agent: mustKnow(agentKindOf(card.agent), "agent"),
    model: card.model,
    thinking: card.think === null ? null : mustKnow(thinkingModeOf(card.think), "thinking mode"),
    permissionMode: mustKnow(permissionModeOf(card.perm), "permission mode"),
    role: card.role,
    labels: wireLabels(card),
    package: card.pkg ?? "",
    plannedStart: atDay(base, card.s),
    plannedEnd: atDay(base, card.e),
    due: atDay(base, card.due),
    pullRequest: card.pr === null ? null : { number: card.pr, url: "" },
    ci: card.ci,
    contextUsed: Math.round(card.ctx * PERCENT),
    needsReason: wireReason(card),
    doingNow: card.doing,
    paused: card.paused,
    pinned: card.pinned,
    session: sessionOf(card),
    branch: card.branch ?? "",
    createdAt: new Date(card.upd).toISOString(),
    updatedAt: new Date(card.upd).toISOString(),
  });

/** The cards grouped into one board per project, which is the unit the mirror applies. */
const boardsOf = (cards: readonly Card[], base: number): DaemonBoard[] => {
  const byProject = new Map<string, WireCard[]>();
  for (const card of cards) {
    const board = byProject.get(card.p) ?? [];
    board.push(wireFrom(card, base));
    byProject.set(card.p, board);
  }
  return [...byProject].map(([projectId, board]) => boardOf(projectId, board));
};

/**
 * Puts the prototype's own values back on a card, for the fields the daemon does not send. The
 * stored session is taken off as well: the store this seeds has no daemon, so its cards are the
 * mock's, with no session of their own (`isAwake` then reads the prototype's rule, and the mock's
 * own sleep and wake, which change `asleep` and `waking` and nothing else, stay consistent). The
 * flags themselves were drawn from the session by the mirror, which is what proves they agree.
 */
const restorePrototypeOnly = (card: Card, source: Card): void => {
  for (const name of PROTOTYPE_ONLY) {
    Object.assign(card, { [name]: structuredClone(source[name]) });
  }
  Object.assign(card, { session: undefined });
};

/** The prototype's 29 cards, built from its own seed data at the moment given. */
const buildPrototypeCards = (loadedAt: number): Card[] => {
  const cards = seedCards(loadedAt);
  // A counter of its own: the store's own numbered the reservoir's checklists and comments when it
  // was built, and the prototype numbered these from one in the same order.
  seedCardExtras(cards, createIds(), loadedAt);
  return cards;
};

/**
 * A test's own copy of the prototype's 29 cards, in the shape the store holds them. It is built
 * from the seed data and the clock at the moment it is called, never from a store, so a test file
 * that captures it at module load works whether S5a is on the mock or on the daemon, and every call
 * is a fresh clone that a test may change.
 */
export const prototypeCards = (): Card[] => structuredClone(buildPrototypeCards(Date.now()));

/**
 * The prototype's 29 cards as the daemon sends them, for a fake daemon a test gives a store. It is
 * the same conversion `applyPrototypeCards` uses below, so a card that arrives from the daemon is
 * the card a test would have seeded.
 */
export const prototypeWireCards = (loadedAt = Date.now()): WireCard[] =>
  prototypeBoards(loadedAt).flatMap((board) => board.cards);

/** The same cards, board by board, which is the shape a snapshot is applied in. */
export const prototypeBoards = (loadedAt = Date.now()): DaemonBoard[] =>
  boardsOf(buildPrototypeCards(loadedAt), startOfDay(loadedAt));

/**
 * Seeds the prototype's 29 cards into a store through the real mirror, for a store whose section
 * table has put S5a on the daemon. The store's cards end up the same as the mock path's, field for
 * field, so flipping the section changes nothing a unit test can see. Their order is the mirror's
 * (by project, then number), which is the order a real board arrives in.
 */
export const applyPrototypeCards = (ctx: Ctx): void => {
  const cards = buildPrototypeCards(ctx.loadedAt);
  applyCardSnapshot(ctx, boardsOf(cards, startOfDay(ctx.clock.now())));
  const source = new Map(cards.map((card) => [card.id, card]));
  for (const card of ctx.S.cards) {
    const prototype = source.get(card.id);
    if (prototype) restorePrototypeOnly(card, prototype);
  }
};
