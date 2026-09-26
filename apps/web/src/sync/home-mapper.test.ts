import type { FeedEntry, HomeStats, Page } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "~/data/testing/golden";
import {
  toStoredDailyStats,
  toStoredFeedItem,
  toStoredFeedItems,
  toStoredStatDay,
} from "./home-mapper";

// The Home numbers and the activity stream, from the daemon's golden files to the shapes the
// screens read (docs/backend-checklist.md B2.3, sections S19a and S20).

const page = golden<Page<FeedEntry>>("home-activity");
const stats = golden<HomeStats>("home-stats");

describe("toStoredFeedItem", () => {
  it("maps a merge entry, with what it opens", () => {
    const entry = page.items[0];
    if (!entry) throw new Error("the golden page has no merge entry");
    expect(toStoredFeedItem(entry)).toEqual({
      id: "01M3C107JB041061050R3GG28A",
      kind: "merge",
      text: "#110 Fix flaky login e2e test merged into main",
      pid: "web",
      ts: Date.UTC(2026, 8, 30, 12, 0, 0, 0),
      cardId: "web#110",
    });
  });

  it("leaves out a card and a job an entry has none of, so a lookup sees no card", () => {
    const brief = page.items[1];
    if (!brief) throw new Error("the golden page has no brief entry");
    const mapped = toStoredFeedItem(brief);
    expect(mapped).toEqual({
      id: "01M3C107JB041061050R3GG28C",
      kind: "brief",
      text: "Morning brief is ready",
      pid: null,
      ts: Date.UTC(2026, 8, 30, 9, 0, 0, 0),
      job: "s1",
    });
    expect("cardId" in mapped).toBe(false);
  });

  it("keeps the order of a page, which is newest first on both sides", () => {
    const kinds = toStoredFeedItems(page.items).map((item) => item.kind);
    expect(kinds).toEqual(["merge", "brief", "ci"]);
  });
});

describe("toStoredDailyStats", () => {
  it("maps the golden range, one row per day and one series per project", () => {
    const mapped = toStoredDailyStats(stats);
    expect(mapped.range).toBe(7);
    expect(mapped.days).toHaveLength(7);
    expect(mapped.days[0]).toEqual({
      day: Date.UTC(2026, 8, 24),
      cardsFinished: 2,
      merges: 2,
      ciFailures: 0,
      costMicros: 0,
    });
    expect(mapped.projects.map((project) => project.projectId)).toEqual(["api", "web"]);
    expect(mapped.projects[0]?.days).toHaveLength(7);
  });

  it("turns a day the daemon did not send into an empty range rather than undefined", () => {
    expect(toStoredDailyStats(null)).toEqual({ range: 0, days: [], projects: [] });
  });

  it("maps one day on its own, with its time in milliseconds", () => {
    const day = stats.days[6];
    if (!day) throw new Error("the golden range has no last day");
    expect(toStoredStatDay(day)).toEqual({
      day: Date.UTC(2026, 8, 30),
      cardsFinished: 3,
      merges: 3,
      ciFailures: 0,
      costMicros: 0,
    });
  });
});
