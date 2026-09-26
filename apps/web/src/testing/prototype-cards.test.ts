/**
 * Section S5a is switching from the mock to the daemon. Once it has, the reservoir keeps the mock's
 * own 29 cards out of the store for good, so the test store seeds them through the real mirror
 * instead (`prototype-cards.ts`). These tests are the promise that the switch is invisible to the
 * suite: the daemon path gives the same 29 cards as the mock path, and the mock path is untouched.
 */
import { describe, expect, it } from "vitest";
import type { Card } from "~/mock/types";
import { prototypeCards } from "./prototype-cards";
import { contextOf, createTestMarshal, DAEMON_CARDS, MOCK_CARDS } from "./test-store";

/** The cards by key, which is the one order both paths agree on: a board sorts by project and number. */
const byKey = (a: Card, b: Card): number => (a.id < b.id ? -1 : 1);

const byId = (M: ReturnType<typeof createTestMarshal>): Card[] => [...M.S.cards].sort(byKey);

/** One card of a set, by its key. It is how a test that cares about a single card names it. */
const cardOf = (cards: readonly Card[], id: string): Card => {
  const card = cards.find((one) => one.id === id);
  if (!card) throw new Error(`the prototype has no card ${id}`);
  return card;
};

/**
 * A card with the fields that count from the clock its store was built at set aside. Two
 * `createTestMarshal` calls happen milliseconds apart, so a card's own `upd`, its checklist items'
 * `doneAt`, and its comments' `ts` differ between the two stores by exactly that much and by
 * nothing else. The cutover cannot change them, so the comparison drops them rather than pretend
 * two wall-clock moments are one.
 */
const withoutClock = (card: Card): Card => ({
  ...card,
  upd: 0,
  // The daemon's own id for the card, which its routes take. The mock path has none, because a mock
  // card never reaches the daemon, so the two paths are compared without it.
  daemonId: undefined,
  // Which view the card opens in (section S9). The daemon says it for every card ("chat" until the
  // person switches to the terminal), and a mock card carries none, so it too is set aside.
  viewMode: undefined,
  checklists: card.checklists.map((list) => ({
    ...list,
    items: list.items.map((item) => ({ ...item, doneAt: 0 })),
  })),
  comments: card.comments.map((comment) => ({ ...comment, ts: 0 })),
});

/** The daemon path, and the mock path, asked for out loud so both keep working after the cutover. */
const CARDS_ON_DAEMON = { sections: DAEMON_CARDS };
const CARDS_ON_MOCK = { sections: MOCK_CARDS };

describe("the prototype's cards through the daemon path", () => {
  it("gives the test store the same 29 cards as the mock path, field for field", () => {
    const mock = createTestMarshal(CARDS_ON_MOCK);
    const onDaemon = createTestMarshal(CARDS_ON_DAEMON);
    expect(onDaemon.S.cards).toHaveLength(29);
    expect(byId(onDaemon).map(withoutClock)).toEqual(byId(mock).map(withoutClock));
  });

  it("leaves the mock path exactly as it was: the reservoir still fills the store", () => {
    const M = createTestMarshal(CARDS_ON_MOCK);
    expect(M.S.cards).toHaveLength(29);
    expect(contextOf(M).hidden.cards).toEqual([]);
  });

  it("keeps the mock's own cards in the reservoir while the section is on the daemon", () => {
    const M = createTestMarshal(CARDS_ON_DAEMON);
    expect(contextOf(M).hidden.cards).toHaveLength(29);
  });

  it("hands a test its own fresh copy of the 29 cards without a store", () => {
    const mock = createTestMarshal();
    const cards = prototypeCards();
    expect(cards).toHaveLength(29);
    expect(cards.map(withoutClock).sort(byKey)).toEqual(byId(mock).map(withoutClock));
    // A test may change its copy: the next call is built again, not shared with the last one.
    cardOf(cards, "api#41").title = "changed by a test";
    expect(cardOf(prototypeCards(), "api#41").title).toBe("Fix token refresh on login");
  });
});
