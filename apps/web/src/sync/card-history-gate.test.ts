import { describe, expect, it } from "vitest";
import { createTestContext, DAEMON_HISTORY, MOCK_CARDS, MOCK_HISTORY } from "~/testing/test-store";

/**
 * The two sections the card panel's history belongs to. Both tables come from the test store, so a
 * test says which side it means rather than relying on the register.
 */
const BOTH_SIDES = { mock: MOCK_HISTORY, daemon: DAEMON_HISTORY };

describe("the mock's chat and activity seed once their sections are the daemon's", () => {
  it("is kept while both sections are still the mock's, as it is at the baseline", () => {
    expect(BOTH_SIDES.mock.S8a).toBe("mock");
    expect(BOTH_SIDES.daemon.S8a).toBe("daemon");
    const ctx = createTestContext({ sections: MOCK_HISTORY });
    expect(Object.keys(ctx.S.chat).length).toBeGreaterThan(0);
    expect(Object.keys(ctx.S.act).length).toBeGreaterThan(0);
    expect(ctx.S.chat["api#41"]?.length).toBeGreaterThan(0);
  });

  it("is not put in the store at all once the chat is the daemon's", () => {
    const ctx = createTestContext({ sections: { ...DAEMON_HISTORY, S10: "mock" } });
    expect(ctx.S.chat).toEqual({});
    expect(Object.keys(ctx.S.act).length).toBeGreaterThan(0);
  });

  it("is not put in the store at all once the activity is the daemon's", () => {
    const ctx = createTestContext({ sections: { ...DAEMON_HISTORY, S8a: "mock" } });
    expect(ctx.S.act).toEqual({});
    expect(Object.keys(ctx.S.chat).length).toBeGreaterThan(0);
  });

  it("leaves neither behind when both are the daemon's, so a real card's history is the only one", () => {
    const ctx = createTestContext({ sections: DAEMON_HISTORY });
    expect(ctx.S.chat).toEqual({});
    expect(ctx.S.act).toEqual({});
  });

  it("is gated on the section and not on the cards, so the two gates stay independent", () => {
    const ctx = createTestContext({ sections: { ...MOCK_CARDS, S8a: "daemon", S10: "daemon" } });
    // The cards are the mock's here, and their chat is not: a card drawn from the mock still reads
    // its conversation from the daemon the moment the chat section switches.
    expect(ctx.S.cards.length).toBeGreaterThan(0);
    expect(ctx.S.chat).toEqual({});
  });
});
