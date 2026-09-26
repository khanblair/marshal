import type { FeedEntry, HomeSnapshot, HomeStats } from "@marshal/protocol";
import { unwrap } from "solid-js/store";
import { describe, expect, it } from "vitest";
import { golden } from "~/data/testing/golden";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { applyHomeStatsEvent, homeStatsSyncer } from "./home-stats";

// Section S19a: the stored numbers behind the Home charts (docs/backend-checklist.md B2.3). The
// syncer reads the daemon's range once and keeps the day a finished card changed current from the
// activity.created events of the home topic.

const stats = golden<HomeStats>("home-stats");
const created = golden<{ entry: FeedEntry }>("activity-created");

const snapshot: HomeSnapshot = {
  needs: [],
  awake: [],
  tiles: { needs: 0, working: 0, mergedToday: 0 },
  stats,
  serverTime: "2026-09-30T12:00:00.000Z",
};

/** The numbers one day's row is given. */
interface Counts {
  cardsFinished: number;
  merges?: number;
  ciFailures?: number;
}

/** The day an activity.created event carries, in the wire's own shape. */
function dayOf(counts: Counts) {
  return {
    day: "2026-09-30T00:00:00.000Z",
    cardsFinished: counts.cardsFinished,
    merges: counts.merges ?? 0,
    ciFailures: counts.ciFailures ?? 0,
    costMicros: 0,
  };
}

/** One activity.created event for a project's own day. */
function eventOf(projectId: string, counts: Counts) {
  return {
    seq: 1,
    topic: "home",
    type: "activity.created",
    at: "2026-09-30T12:00:00.000Z",
    data: { entry: { ...created.entry, projectId }, day: dayOf(counts) },
  } as never;
}

describe("the Home numbers section", () => {
  it("is section S19a, follows the home topic, and reads the activity events", () => {
    expect(homeStatsSyncer.section).toBe("S19a");
    expect(homeStatsSyncer.topics).toEqual(["home"]);
    expect(homeStatsSyncer.onEvent).toBeTypeOf("function");
  });

  it("maps the daemon's stored range into the store, one day per point", () => {
    const ctx = contextOf(createTestMarshal());
    homeStatsSyncer.apply(ctx, snapshot);
    expect(ctx.S.stats.range).toBe(7);
    expect(ctx.S.stats.days.map((day) => day.day)).toEqual([
      Date.UTC(2026, 8, 24),
      Date.UTC(2026, 8, 25),
      Date.UTC(2026, 8, 26),
      Date.UTC(2026, 8, 27),
      Date.UTC(2026, 8, 28),
      Date.UTC(2026, 8, 29),
      Date.UTC(2026, 8, 30),
    ]);
    expect(ctx.S.stats.days[6]).toMatchObject({ cardsFinished: 3, merges: 3 });
    expect(ctx.S.stats.projects.map((project) => project.projectId)).toEqual(["api", "web"]);
  });

  it("sets the whole day of the project a card finished in, and the totals with it", () => {
    const ctx = contextOf(createTestMarshal());
    homeStatsSyncer.apply(ctx, snapshot);
    // The event carries the project's own day as it is now, not a difference.
    applyHomeStatsEvent(ctx, eventOf("api", { cardsFinished: 4, merges: 4, ciFailures: 1 }));

    const api = ctx.S.stats.projects.find((project) => project.projectId === "api");
    expect(api?.days[6]).toEqual({
      day: Date.UTC(2026, 8, 30),
      cardsFinished: 4,
      merges: 4,
      ciFailures: 1,
      costMicros: 0,
    });
    // The totals are the projects added together: api's 4 and web's 2.
    expect(ctx.S.stats.days[6]).toEqual({
      day: Date.UTC(2026, 8, 30),
      cardsFinished: 6,
      merges: 6,
      ciFailures: 1,
      costMicros: 0,
    });
    // A day the event did not touch keeps what the snapshot gave it.
    expect(ctx.S.stats.days[0]).toMatchObject({ cardsFinished: 2, merges: 2 });
  });

  it("changes nothing when the same event is applied twice, or after a reload", () => {
    const ctx = contextOf(createTestMarshal());
    homeStatsSyncer.apply(ctx, snapshot);
    const event = eventOf("api", { cardsFinished: 4, merges: 4, ciFailures: 1 });
    applyHomeStatsEvent(ctx, event);
    const once = unwrap(ctx.S.stats);
    applyHomeStatsEvent(ctx, event);
    expect(ctx.S.stats).toEqual(once);
  });

  it("starts a project's series when the project has no stored day yet", () => {
    const ctx = contextOf(createTestMarshal());
    homeStatsSyncer.apply(ctx, snapshot);
    applyHomeStatsEvent(ctx, eventOf("billing", { cardsFinished: 1, merges: 1 }));

    const billing = ctx.S.stats.projects.find((project) => project.projectId === "billing");
    expect(billing?.days).toHaveLength(7);
    expect(billing?.days[0]).toEqual({
      day: Date.UTC(2026, 8, 24),
      cardsFinished: 0,
      merges: 0,
      ciFailures: 0,
      costMicros: 0,
    });
    expect(billing?.days[6]).toMatchObject({ cardsFinished: 1, merges: 1 });
    // The totals now count the new project too.
    expect(ctx.S.stats.days[6]).toMatchObject({ cardsFinished: 4 });
  });

  it("ignores a change that comes before the range was loaded, since the load brings it", () => {
    const ctx = contextOf(createTestMarshal());
    const before = ctx.S.stats;
    applyHomeStatsEvent(ctx, eventOf("api", { cardsFinished: 4, merges: 4 }));
    expect(ctx.S.stats).toBe(before);
  });

  it("ignores an entry that belongs to no project, and a row that changed no numbers", () => {
    const ctx = contextOf(createTestMarshal());
    homeStatsSyncer.apply(ctx, snapshot);
    const before = unwrap(ctx.S.stats);
    applyHomeStatsEvent(ctx, {
      seq: 2,
      topic: "home",
      type: "activity.created",
      at: "2026-09-30T12:00:00.000Z",
      data: { entry: { ...created.entry, projectId: null }, day: dayOf({ cardsFinished: 1 }) },
    } as never);
    applyHomeStatsEvent(ctx, {
      seq: 3,
      topic: "home",
      type: "activity.created",
      at: "2026-09-30T12:00:00.000Z",
      data: { entry: created.entry, day: null },
    } as never);
    applyHomeStatsEvent(ctx, { type: "card.moved", data: { card: {} } } as never);
    expect(ctx.S.stats).toEqual(before);
  });
});
