import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type FeedItem, M } from "~/mock";
import { feedIcon, feedProjectName, openFeedItem } from "./feed-row";
import { type HomeSnapshot, homeSnapshot, resetHome } from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const snapshot: HomeSnapshot = homeSnapshot();

beforeEach(() => {
  vi.useFakeTimers();
  resetHome(snapshot);
});
afterEach(() => {
  vi.useRealTimers();
});

const entry = (patch: Partial<FeedItem>): FeedItem => ({
  id: "f-test",
  kind: "tool",
  text: "Something happened",
  pid: null,
  ts: 0,
  ...patch,
});

describe("feedIcon", () => {
  it.each([
    ["merge", "git-merge", "var(--color-status-ready-solid)"],
    ["approval", "st-needs", "var(--color-status-needs-you-solid)"],
    ["plan", "list-checks", "var(--color-text-secondary)"],
    ["schedule", "clock", "var(--color-text-secondary)"],
    ["brief", "sunrise", "var(--color-text-secondary)"],
    ["tool", "info", "var(--color-text-secondary)"],
  ] as const)("draws a %s entry as %s", (kind, name, color) => {
    expect(feedIcon(entry({ kind }))).toEqual({ name, color });
  });

  it("draws CI results red when they failed and green otherwise", () => {
    expect(feedIcon(entry({ kind: "ci", text: "CI failed on main" }))).toEqual({
      name: "circle-x",
      color: "var(--color-status-danger-solid)",
    });
    expect(feedIcon(entry({ kind: "ci", text: "CI passed on main" }))).toEqual({
      name: "circle-check",
      color: "var(--color-status-working-solid)",
    });
  });

  it("draws a kind it does not know as a plain note", () => {
    expect(feedIcon(entry({ kind: "unknown" as never })).name).toBe("info");
  });
});

describe("feedProjectName", () => {
  it("names the project, or nothing for no project or a removed one", () => {
    expect(feedProjectName(entry({ pid: "web" }))).toBe("web-dashboard");
    expect(feedProjectName(entry({ pid: null }))).toBe("");
    expect(feedProjectName(entry({ pid: "gone" }))).toBe("");
  });
});

describe("openFeedItem", () => {
  it("opens the card first", () => {
    openFeedItem(entry({ cardId: "api#43", pid: "api", job: "s1" }));
    expect(M.S.openId).toBe("api#43");
    expect(M.S.route.page).toBe("home");
  });

  it("opens the schedule when the card is gone", () => {
    openFeedItem(entry({ cardId: "api#99999", job: "s3", pid: "web" }));
    expect(M.S.route.page).toBe("settings");
    expect(M.S.settingsSection).toBe("schedules");
    expect(M.S.schedEdit).toBe("s3");
  });

  it("opens the project when there is no card or schedule", () => {
    openFeedItem(entry({ pid: "mobile" }));
    expect(M.S.route).toMatchObject({ page: "project", pid: "mobile" });
  });

  it("does nothing for an entry about nothing", () => {
    openFeedItem(entry({ pid: "gone" }));
    expect(M.S.route.page).toBe("home");
    expect(M.S.openId).toBeNull();
  });
});
