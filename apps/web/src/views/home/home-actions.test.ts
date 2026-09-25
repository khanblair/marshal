import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import {
  mergedTodayCount,
  openAgents,
  openCalendar,
  openCostLimits,
  openStatusList,
  viewAllActivity,
  viewAllCi,
} from "./home-actions";
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

describe("mergedTodayCount", () => {
  it("counts today's merges in the feed", () => {
    M.S.feed = [];
    expect(mergedTodayCount()).toBe(0);
    M.S.feed.push(
      { id: "a", kind: "merge", text: "#1 merged", pid: "api", cardId: "api#1", ts: M.T0 + 1 },
      { id: "b", kind: "merge", text: "#2 merged", pid: "api", cardId: "api#2", ts: M.T0 - 1 },
      { id: "c", kind: "ci", text: "CI passed", pid: "api", ts: M.T0 + 5 },
    );
    expect(mergedTodayCount()).toBe(1);
  });

  it("counts done cards updated today that have no merge entry, once", () => {
    M.S.feed = [];
    const [first, second] = M.S.cards.filter((c) => c.state === "done");
    if (!first || !second) throw new Error("seed has too few done cards");
    first.upd = M.T0 + 10;
    second.upd = M.T0 + 20;
    expect(mergedTodayCount()).toBe(2);
    M.S.feed.push({
      id: "m",
      kind: "merge",
      text: "merged",
      pid: first.p,
      cardId: first.id,
      ts: M.T0 + 15,
    });
    expect(mergedTodayCount()).toBe(2);
  });
});

describe("openStatusList", () => {
  it("opens the list of the project with the most cards in that column, filtered to it", () => {
    const counts = M.S.projects.map(
      (p) =>
        [
          p.id,
          M.S.cards.filter((c) => c.p === p.id && M.colOf(c.state) === "working").length,
        ] as const,
    );
    const best = [...counts].sort((a, b) => b[1] - a[1])[0]?.[0];
    openStatusList("working");
    expect(M.S.route).toMatchObject({ page: "project", pid: best, view: "list" });
    expect(M.S.filters[best ?? ""]).toEqual([{ k: "status", v: "working" }]);
    expect(M.S.savedView[best ?? ""]).toBeNull();
  });

  it("does nothing without projects", () => {
    M.S.projects = [];
    openStatusList("needs");
    expect(M.S.route.page).toBe("home");
  });
});

describe("links", () => {
  it("opens the cost limits", () => {
    openCostLimits();
    expect(M.S).toMatchObject({ settingsSection: "limits", route: { page: "settings" } });
  });

  it("opens the two view-all pages", () => {
    viewAllCi();
    expect(M.S).toMatchObject({ allKind: "ci", route: { page: "all" } });
    viewAllActivity();
    expect(M.S.allKind).toBe("activity");
  });

  it("opens the current project's calendar, else the busiest project's", () => {
    M.S.route = { page: "home", pid: "web", view: "board" };
    openCalendar();
    expect(M.S.route).toMatchObject({ pid: "web", view: "calendar" });
    M.S.route = { page: "home", pid: "gone", view: "board" };
    openCalendar();
    const busiest = [...M.S.projects].sort(
      (a, b) => M.awake(b.id).length - M.awake(a.id).length,
    )[0];
    expect(M.S.route).toMatchObject({ pid: busiest?.id, view: "calendar" });
  });

  it("opens the agents of the busiest project", () => {
    const busiest = [...M.S.projects].sort(
      (a, b) => M.awake(b.id).length - M.awake(a.id).length,
    )[0];
    openAgents();
    expect(M.S.route).toMatchObject({ page: "project", pid: busiest?.id, view: "agents" });
  });
});
