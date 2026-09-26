import { describe, expect, it } from "vitest";
import type { ActivityCreatedEventData, FeedEntry, HomeStatDay, HomeStats, Page } from "../src";
import { golden } from "./golden";

// The stored Home numbers and the activity stream (docs/backend-checklist.md B2.3, sections S19a
// and S20). The samples are checked against the generated types by the compiler and against the
// daemon's own files by the assertions.
describe("the Home numbers and the activity stream", () => {
  /** One stored day. Cost is zero on every stored day until Phase 4 fills it. */
  const stat = (
    day: string,
    cardsFinished: number,
    merges: number,
    ciFailures: number,
  ): HomeStatDay => ({ day, cardsFinished, merges, ciFailures, costMicros: 0 });

  it("has a week of stored days, all projects and one series each", () => {
    const sample: HomeStats = {
      range: 7,
      from: "2026-09-24T00:00:00.000Z",
      to: "2026-09-30T00:00:00.000Z",
      days: [
        stat("2026-09-24T00:00:00.000Z", 2, 2, 0),
        stat("2026-09-25T00:00:00.000Z", 3, 3, 0),
        stat("2026-09-26T00:00:00.000Z", 1, 1, 1),
        stat("2026-09-27T00:00:00.000Z", 4, 3, 0),
        stat("2026-09-28T00:00:00.000Z", 2, 2, 0),
        stat("2026-09-29T00:00:00.000Z", 5, 4, 2),
        stat("2026-09-30T00:00:00.000Z", 3, 3, 0),
      ],
      projects: [
        {
          projectId: "api",
          days: [
            stat("2026-09-24T00:00:00.000Z", 1, 1, 0),
            stat("2026-09-25T00:00:00.000Z", 2, 2, 0),
            stat("2026-09-26T00:00:00.000Z", 0, 0, 1),
            stat("2026-09-27T00:00:00.000Z", 2, 2, 0),
            stat("2026-09-28T00:00:00.000Z", 1, 1, 0),
            stat("2026-09-29T00:00:00.000Z", 3, 2, 2),
            stat("2026-09-30T00:00:00.000Z", 1, 1, 0),
          ],
        },
        {
          projectId: "web",
          days: [
            stat("2026-09-24T00:00:00.000Z", 1, 1, 0),
            stat("2026-09-25T00:00:00.000Z", 1, 1, 0),
            stat("2026-09-26T00:00:00.000Z", 1, 1, 0),
            stat("2026-09-27T00:00:00.000Z", 2, 1, 0),
            stat("2026-09-28T00:00:00.000Z", 1, 1, 0),
            stat("2026-09-29T00:00:00.000Z", 2, 2, 0),
            stat("2026-09-30T00:00:00.000Z", 2, 2, 0),
          ],
        },
      ],
    };
    expect(golden("home-stats")).toEqual(sample);
    // The totals are the projects added together, which is what a chart that draws either shows.
    const today = sample.days.at(-1);
    const projectsToday = sample.projects.map((project) => project.days.at(-1)?.cardsFinished ?? 0);
    expect(projectsToday.reduce((sum, value) => sum + value, 0)).toBe(today?.cardsFinished);
  });

  it("has an empty range as day rows, never null", () => {
    const empty: HomeStats = {
      range: 30,
      from: "2026-09-01T00:00:00.000Z",
      to: "2026-09-30T00:00:00.000Z",
      days: [],
      projects: [],
    };
    expect(golden("home-stats-empty")).toEqual(empty);
  });

  it("has a page of the activity stream, with what each row opens", () => {
    const sample: Page<FeedEntry> = {
      items: [
        {
          id: "01M3C107JB041061050R3GG28A",
          kind: "merge",
          text: "#110 Fix flaky login e2e test merged into main",
          projectId: "web",
          at: "2026-09-30T12:00:00.000Z",
          cardId: "01M3C107JB041061050R3GG28B",
          cardKey: "web#110",
          jobId: "",
        },
        {
          id: "01M3C107JB041061050R3GG28C",
          kind: "brief",
          text: "Morning brief is ready",
          projectId: null,
          at: "2026-09-30T09:00:00.000Z",
          cardId: "",
          cardKey: "",
          jobId: "s1",
        },
        {
          id: "01M3C107JB041061050R3GG28D",
          kind: "ci",
          text: "CI passed on marshal/39-slog",
          projectId: "api",
          at: "2026-09-30T08:30:00.000Z",
          cardId: "01M3C107JB041061050R3GG28E",
          cardKey: "api#39",
          jobId: "",
        },
      ],
      nextCursor: "eyJzZXEiOjF9",
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("home-activity")).toEqual(sample);
    // A row about no project carries none, so the app can show it without a project name.
    expect(sample.items[1]?.projectId).toBeNull();
  });

  it("has the event that appends a row, with the day it changed", () => {
    const sample: ActivityCreatedEventData = {
      entry: {
        id: "01M3C107JB041061050R3GG28A",
        kind: "merge",
        text: "#110 Fix flaky login e2e test merged into main",
        projectId: "web",
        at: "2026-09-30T12:00:00.000Z",
        cardId: "01M3C107JB041061050R3GG28B",
        cardKey: "web#110",
        jobId: "",
      },
      day: {
        day: "2026-09-30T00:00:00.000Z",
        cardsFinished: 3,
        merges: 3,
        ciFailures: 0,
        costMicros: 0,
      },
    };
    expect(golden("activity-created")).toEqual(sample);

    const withoutDay: ActivityCreatedEventData = {
      entry: {
        id: "01M3C107JB041061050R3GG28C",
        kind: "brief",
        text: "Morning brief is ready",
        projectId: null,
        at: "2026-09-30T12:00:00.000Z",
        cardId: "",
        cardKey: "",
        jobId: "s1",
      },
      day: null,
    };
    expect(golden("activity-created-no-day")).toEqual(withoutDay);
  });
});
