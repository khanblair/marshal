import {
  EventTypeCardCreated,
  EventTypeCardDeleted,
  EventTypeCardMoved,
  EventTypeCardUpdated,
  type Card as WireCard,
  type Event as WireEvent,
} from "@marshal/protocol";
import { batch } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import type { MirroredCard } from "~/data/mappers/card";
import { mirroredCard, toStoredCard } from "~/data/mappers/card";
import type { CardKey } from "~/mock/card-key";
import type { Ctx } from "~/mock/context";
import type { Card } from "~/mock/types";
import type { Syncer } from "./syncer";

/** One project's board, as the daemon answered it. */
export interface DaemonBoard {
  projectId: string;
  cards: WireCard[];
}

/**
 * Section S5a (and the card parts of S7a and S18, which draw the same cards): every project's
 * board, and the `card.*` events of every project's topic.
 *
 * A card's events go to `project:<id>`, so `projectTopics` asks for each project the store holds:
 * that is what the extended `Syncer` is for. A card's own events (its messages and session output)
 * go to `card:<id>`, which only the open card needs, so that subscription belongs to the card panel
 * and not here.
 */
export const cardsSyncer: Syncer<DaemonBoard[]> = {
  section: "S5a",
  topics: [],
  projectTopics: (projectID) => [`project:${projectID}`],
  async load(api: ApiClient, ctx: Ctx) {
    // One board per project. There is no call that lists cards across projects.
    const projects = ctx.S.projects.map((project) => project.id);
    return Promise.all(
      projects.map(async (projectId) => ({
        projectId,
        cards: (await api.board(projectId)).cards,
      })),
    );
  },
  apply: applyCardSnapshot,
  onEvent: applyCardEvent,
};

/**
 * A card as the store keeps it: what the daemon fills, plus the fields that are the app's own. Those
 * are empty on a real card, because the daemon has nothing to say about them yet: cost and token use
 * are Phase 4, dependencies, members, checklists, and comments are later phases, and the merge
 * percent and the bypass flag belong to the phases that build them. They are written here, once, and
 * never written from the wire afterwards.
 *
 * The session is not one of them. The wire card carries its session state (section S7c), so the
 * session and the two flags drawn from it (`asleep` and `waking`) come from the mapper and are
 * rewritten by every board and every `card.updated`.
 */
function storedCard(card: WireCard, now: number): Card {
  return {
    ...toStoredCard(card, now),
    cost: 0,
    deps: [],
    members: [],
    checklists: [],
    comments: [],
    bypass: false,
    mergePct: 0,
  };
}

/**
 * Makes the store's cards the daemon's, board by board. A card that is already there is updated in
 * place (a card that did not change is not redrawn), a new one is added, and a card the daemon no
 * longer lists is forgotten. Applying the same boards twice changes nothing.
 */
export function applyCardSnapshot(ctx: Ctx, boards: readonly DaemonBoard[]): void {
  const { S } = ctx;
  const now = ctx.clock.now();
  batch(() => {
    for (const board of boards) {
      const known = new Map(
        S.cards.filter((card) => card.p === board.projectId).map((card) => [card.id, card]),
      );
      const wanted = new Set(board.cards.map((card) => card.key));
      for (const card of board.cards) {
        const existing = known.get(card.key);
        if (existing) mirrorCard(existing, card, now);
        else S.cards.push(storedCard(card, now));
      }
      for (const [key, card] of known) {
        if (!wanted.has(key)) forgetCard(ctx, card);
      }
    }
    sortCards(ctx);
  });
}

/** One card created or changed on the daemon. */
export function applyCard(ctx: Ctx, wire: WireCard): void {
  const { S } = ctx;
  const now = ctx.clock.now();
  batch(() => {
    const existing = S.cards.find((card) => card.id === wire.key);
    if (existing) mirrorCard(existing, wire, now);
    else S.cards.push(storedCard(wire, now));
    sortCards(ctx);
  });
}

/** A card removed on the daemon, by its key. It does nothing when the store already forgot it. */
export function applyCardRemoved(ctx: Ctx, key: string): void {
  const card = ctx.S.cards.find((c) => c.id === key);
  if (card) forgetCard(ctx, card);
}

/** Writes the daemon's values into a card that is already in the store, one field at a time. */
function mirrorCard(target: Card, wire: WireCard, now: number): void {
  const next: MirroredCard = mirroredCard(wire, now);
  // One field at a time, so a field that changed redraws and one that did not, does not. Each key
  // is written through its own type, which is why this is not one object assignment.
  for (const key of Object.keys(next) as (keyof typeof next)[]) {
    if (target[key] !== next[key]) {
      Object.assign(target, { [key]: next[key] });
    }
  }
}

/**
 * The per-card maps the store keeps. A card that is gone leaves nothing behind in any of them, and
 * a new card can take its number, so the key must not carry the old card's records.
 */
const CARD_MAPS = ["chat", "act", "checks", "notes", "preview"] as const;

/** Drops a card and everything the store keeps for it. */
function forgetCard(ctx: Ctx, card: Card): void {
  forgetCardKey(ctx, card.id);
}

/** Drops everything the store keeps for a card key, whether or not the card is still there. */
function forgetCardKey(ctx: Ctx, key: CardKey): void {
  batch(() => {
    ctx.S.cards = ctx.S.cards.filter((c) => c.id !== key);
    if (ctx.S.openId === key) ctx.S.openId = null;
    if (ctx.S.focusId === key) ctx.S.focusId = null;
    for (const name of CARD_MAPS) {
      const perCard = ctx.S[name] as Record<string, unknown> | undefined;
      if (perCard) delete perCard[key];
    }
  });
}

/**
 * Keeps the board's order: by project, then by number, which is the order the daemon answers in and
 * the order the boards draw.
 */
function sortCards(ctx: Ctx): void {
  const cards = [...ctx.S.cards].sort((a, b) => (a.p === b.p ? a.n - b.n : a.p < b.p ? -1 : 1));
  if (cards.some((card, i) => card !== ctx.S.cards[i])) ctx.S.cards = cards;
}

/** The `card.*` events of a project's topic. Each carries the card as it is now. */
function applyCardEvent(ctx: Ctx, event: WireEvent): void {
  if (event.type === EventTypeCardDeleted) {
    // The payload carries the card's key, which is what the store is keyed by: a client cannot turn
    // the opaque id back into a key on its own.
    if (isRecord(event.data) && typeof event.data.key === "string") {
      forgetCardKey(ctx, event.data.key);
    }
    return;
  }
  if (
    event.type !== EventTypeCardCreated &&
    event.type !== EventTypeCardUpdated &&
    event.type !== EventTypeCardMoved
  ) {
    return;
  }
  if (isRecord(event.data) && isRecord(event.data.card)) {
    applyCard(ctx, event.data.card as unknown as WireCard);
  }
}
