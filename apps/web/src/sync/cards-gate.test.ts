import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import { startSimulation } from "~/mock/sim/start";
import {
  contextOf,
  createTestContext,
  createTestMarshal,
  DAEMON_CARDS,
} from "~/testing/test-store";
import { reconcileMock } from "./reservoir";

/** The two paths, asked for out loud, so both keep working after the cards cut over. */
const CARDS_ON_DAEMON = DAEMON_CARDS;
const CARDS_ON_MOCK = { ...sectionStatus, S5a: "mock" as const };

describe("the mock's cards once S5a is on the daemon", () => {
  it("are never brought back, however many projects exist", () => {
    // A store that has not loaded a board yet is what the gate is for: the prototype's projects are
    // there, so before the gate `reconcileMock` would have shown the mock's 29 cards as if they were
    // the daemon's. `backend-checklist.md` line 310 is where that is written down.
    const ctx = createTestContext({ sections: CARDS_ON_DAEMON });
    ctx.S.cards = [];
    reconcileMock(ctx);
    reconcileMock(ctx);
    expect(ctx.S.projects.map((project) => project.id)).toEqual(["api", "web", "mobile"]);
    expect(ctx.S.cards).toEqual([]);
    expect(ctx.hidden.cards).toHaveLength(29);
  });

  it("are not the cards the test store seeds through the daemon path either", () => {
    // The test store gives a daemon-path test the prototype's cards through the real mirror
    // (`testing/prototype-cards.ts`). Those are the daemon's cards: the reservoir's own objects must
    // never be among them, or the two paths would share state and a test could pass by accident.
    const M = createTestMarshal({ sections: CARDS_ON_DAEMON });
    expect(M.S.cards).toHaveLength(29);
    expect(contextOf(M).hidden.cards).toHaveLength(29);
    const reservoir = new Set(contextOf(M).hidden.cards);
    expect(M.S.cards.some((card) => reservoir.has(card))).toBe(false);
  });

  it("are still shown while the section is on the mock, as they are at the baseline", () => {
    const M = createTestMarshal({ sections: CARDS_ON_MOCK });
    expect(M.S.cards).toHaveLength(29);
    expect(contextOf(M).hidden.cards).toEqual([]);
  });

  it("come back for a project the daemon adds while the section is on the mock", () => {
    const ctx = createTestContext({ sections: CARDS_ON_MOCK });
    const api = ctx.S.projects.find((project) => project.id === "api")!;
    ctx.S.projects = ctx.S.projects.filter((project) => project.id !== "api");
    reconcileMock(ctx);
    expect(ctx.S.cards.some((card) => card.p === "api")).toBe(false);
    ctx.S.projects = [...ctx.S.projects, api];
    reconcileMock(ctx);
    expect(ctx.S.cards.some((card) => card.p === "api")).toBe(true);
  });
});

describe("the simulation once S5a is on the daemon", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("schedules nothing at all, so no scripted card, merge, CI failure, or tick runs", () => {
    const ctx = createTestContext({ sections: CARDS_ON_DAEMON });
    startSimulation(ctx, "");
    expect(vi.getTimerCount()).toBe(0);
  });

  it("leaves a card alone even when one is in the store, which a bug would otherwise rewrite", () => {
    const ctx = createTestContext({ sections: CARDS_ON_DAEMON });
    const card = ctx.hidden.cards.find((one) => one.id === "api#41")!;
    ctx.S.cards = [card];
    const before = { ...card };
    startSimulation(ctx, "");
    vi.advanceTimersByTime(135_000);
    expect(ctx.S.cards[0]).toEqual(before);
  });

  it("still runs while the section is on the mock, as it does at the baseline", () => {
    const ctx = createTestContext({ sections: CARDS_ON_MOCK });
    startSimulation(ctx, "");
    expect(vi.getTimerCount()).toBeGreaterThan(0);
  });
});
