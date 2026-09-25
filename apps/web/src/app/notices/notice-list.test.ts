import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { buildNotices, countdown, type NoticeModel } from "./notice-list";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const seedNotices = JSON.parse(JSON.stringify(M.S.notices));
const DESKTOP_PX = 1440;
const HEIGHT_PX = 900;
const SLEEP_SECONDS = 125;
const MS = 1000;

function byKey(list: NoticeModel[], key: string): NoticeModel {
  const found = list.find((n) => n.key === key);
  if (!found) throw new Error(`no notice ${key}`);
  return found;
}

beforeEach(() => {
  M.S.notices = structuredClone(seedNotices);
  M.setViewport(DESKTOP_PX, HEIGHT_PX);
  M.go("home");
  M.set({ noticesOpen: false, openId: null, settingsSection: "general" });
});
afterEach(() => vi.restoreAllMocks());

describe("countdown", () => {
  it("writes the time left as m:ss, rounding up", () => {
    expect(countdown(SLEEP_SECONDS * MS, 0)).toBe("2:05");
    expect(countdown(SLEEP_SECONDS * MS, 500)).toBe("2:05");
    expect(countdown(59_001, 0)).toBe("1:00");
  });

  it("stops at zero once the deadline has passed", () => {
    expect(countdown(1000, 5000)).toBe("0:00");
  });
});

describe("buildNotices", () => {
  it("lists the needs-you group first, then the store's notices in order", () => {
    const list = buildNotices(M, false);
    expect(list.map((n) => n.key)).toEqual(["needs", "n1", "n2", "n3"]);
    const needs = byKey(list, "needs");
    expect(needs.title).toBe("4 cards need you");
    expect(needs.sub).toBe("Across 3 projects");
    expect(needs.when).toMatch(/^Oldest /);
    expect(needs.dismiss).toBeUndefined();
    expect(needs.tone).toBe("needs");
  });

  it("gives each waiting card an Open button, plus Approve or Review plan when it applies", () => {
    const rows = byKey(buildNotices(M, false), "needs").rows;
    const labels = (title: string) =>
      rows.find((r) => r.title.startsWith(title))?.actions.map((a) => a.label);
    expect(labels("#43")).toEqual(["Review plan", "Open"]);
    expect(labels("#44")).toEqual(["Approve", "Open"]);
    expect(labels("#119")).toEqual(["Open"]);
    expect(rows.find((r) => r.title.startsWith("#43"))?.subTone).toBe("needs");
    expect(rows.find((r) => r.title.startsWith("#44"))?.actions[0]?.primary).toBe(true);
  });

  it("approves a card from its row", () => {
    const approve = vi.spyOn(M, "approve").mockImplementation(() => {});
    const row = byKey(buildNotices(M, false), "needs").rows.find((r) => r.title.startsWith("#44"));
    row?.actions[0]?.run();
    expect(approve).toHaveBeenCalledWith(44);
  });

  it("writes the sleep countdown from the store clock and lists the idle cards", () => {
    const sleep = byKey(buildNotices(M, false), "n1");
    expect(sleep.title).toMatch(/^3 cards are idle and will sleep in \d+:\d\d$/);
    expect(sleep.dismiss).toBeUndefined();
    expect(sleep.tone).toBe("secondary");
    expect(sleep.rows.map((r) => r.sub)).toEqual(["api-gateway", "api-gateway", "web-dashboard"]);
    expect(sleep.rows[0]?.actions.map((a) => a.label)).toEqual(["Keep awake", "Sleep now", "Pin"]);
    expect(sleep.actions.map((a) => a.label)).toEqual(["Keep all awake", "Sleep all now"]);
  });

  it("says card in the singular for one idle card, and skips a card that no longer exists", () => {
    M.S.notices = [
      { id: "s", kind: "sleep", cards: [39], deadline: Date.now() + MS * 30, ts: Date.now() },
      { id: "t", kind: "sleep", cards: [39, 999999], deadline: Date.now(), ts: Date.now() },
    ];
    const [one, two] = buildNotices(M, false).filter((n) => n.key !== "needs");
    expect(one?.title).toMatch(/^1 card is idle and will sleep in /);
    expect(two?.rows).toHaveLength(1);
  });

  it("marks CI failures as danger and cost warnings as needs-you", () => {
    const list = buildNotices(M, false);
    const ci = byKey(list, "n2");
    expect(ci).toMatchObject({
      icon: "circle-x",
      tone: "danger",
      title: "Main is failing in mobile-app",
    });
    expect(ci.dismiss).toBeTypeOf("function");
    expect(ci.actions.map((a) => [a.label, a.primary])).toEqual([["Open card", true]]);
    const cost = byKey(list, "n3");
    expect(cost).toMatchObject({ icon: "circle-dollar-sign", tone: "needs" });
    expect(cost.actions.map((a) => a.label)).toEqual(["Open limits"]);
  });

  it("uses the plan icon and Review plan for a plan notice, and no button without a card", () => {
    M.S.notices = [
      { id: "p", kind: "plan", cardId: 43, text: "Plan ready", sub: "#43", ts: Date.now() },
      { id: "q", kind: "ci", text: "CI failed", sub: "", ts: Date.now() },
    ];
    const [plan, ci] = buildNotices(M, false).filter((n) => n.key !== "needs");
    expect(plan).toMatchObject({ icon: "st-needs", tone: "needs" });
    expect(plan?.actions.map((a) => a.label)).toEqual(["Review plan"]);
    expect(ci?.actions).toEqual([]);
  });

  it("dismisses a notice through the store", () => {
    byKey(buildNotices(M, false), "n2").dismiss?.();
    expect(M.S.notices.map((n) => n.id)).toEqual(["n1", "n3"]);
  });

  it("opens a card and closes the panel", () => {
    M.set({ noticesOpen: true });
    byKey(buildNotices(M, false), "n2").actions[0]?.run();
    expect(M.S.noticesOpen).toBe(false);
    expect(M.S.openId).toBe(213);
  });

  it("goes to the card's board first on a phone that is not on a project page", () => {
    M.set({ noticesOpen: true });
    byKey(buildNotices(M, true), "n2").actions[0]?.run();
    expect(M.S.route).toMatchObject({ page: "project", pid: "mobile", view: "board" });
    expect(M.S.openId).toBe(213);
  });

  it("opens the limits settings from a cost notice", () => {
    M.set({ noticesOpen: true });
    byKey(buildNotices(M, false), "n3").actions[0]?.run();
    expect(M.S.settingsSection).toBe("limits");
    expect(M.S.noticesOpen).toBe(false);
    expect(M.S.route.page).toBe("settings");
  });

  it("has no needs-you group when nothing waits", () => {
    vi.spyOn(M, "needs").mockReturnValue([]);
    expect(buildNotices(M, false).map((n) => n.key)).toEqual(["n1", "n2", "n3"]);
  });
});
