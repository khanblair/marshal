import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FIXED_TIME, makeTwin, SLOW_TEST_MS, type Twin } from "../testing/twin";
import type { Card } from "../types";

const cardOf = (t: Twin, id: number): Card => {
  const c = t.port.card(id);
  if (!c) throw new Error(`no card ${id}`);
  return c;
};

describe("checklists", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it("toggles, adds, and removes items", () => {
    const list = cardOf(t, 41).checklists[0];
    const item = list?.items[3];
    t.run("toggleItem", 41, list?.id, item?.id);
    t.same();
    expect(cardOf(t, 41).checklists[0]?.items[3]).toMatchObject({ done: true, by: "ada" });
    t.run("toggleItem", 41, list?.id, item?.id);
    t.run("addItem", 41, list?.id, "  Update the changelog ");
    t.run("addItem", 41, list?.id, "  ");
    t.run("removeItem", 41, list?.id, list?.items[0]?.id);
    t.same();
    expect(cardOf(t, 41).checklists[0]?.items.map((i) => i.text)).toContain("Update the changelog");
    // Unknown ids throw in the prototype; the port ignores them.
    t.port.toggleItem(41, "nope", "nope");
    t.port.removeItem(41, "nope", "nope");
    t.port.addItem(41, "nope", "x");
    t.port.toggleHideDone(41, "nope");
    t.port.addChecklist(9999, "x");
    t.same();
  });

  it("adds a checklist, hides done items, and deletes it after confirming", () => {
    t.run("addChecklist", 42, "  Rollout ");
    t.run("addChecklist", 42, "");
    t.same();
    const lists = cardOf(t, 42).checklists;
    expect(lists.map((l) => l.title)).toEqual(["Rollout", "Checklist"]);
    t.run("toggleHideDone", 42, lists[0]?.id);
    t.same();
    t.run("deleteChecklist", 42, lists[0]?.id);
    expect(t.port.S.dialog?.message).toBe('This deletes "Rollout" and its 0 items.');
    t.confirmDialog();
    t.same();
    expect(cardOf(t, 42).checklists).toHaveLength(1);
    t.port.deleteChecklist(42, "nope");
    expect(t.port.S.dialog).toBeNull();
  });
});

describe("comments and members", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it.each([
    [41, "Can you also check https://example.com/spec?", []],
    [41, "@agent see the attached log", [{ kind: "file", name: "log.txt", size: "2 KB" }]],
    [41, "Looks good", []],
    [45, "Waiting for the spec?", []],
    [41, "", [{ kind: "image", name: "a.png", size: "1 KB" }]],
  ])("comments on #%i: %s", (id, text, att) => {
    t.run("addComment", id, text, att);
    t.run("addComment", id, "   ", []);
    t.same();
    t.play(3500, 100);
  });

  it("marks the comment read through the live store object", () => {
    t.port.addComment(41, "Is this done?", []);
    const mine = () => cardOf(t, 41).comments.find((c) => c.text === "Is this done?");
    expect(mine()?.read).toBe(false);
    vi.advanceTimersByTime(1200);
    expect(mine()?.read).toBe(true);
    vi.advanceTimersByTime(1600);
    expect(cardOf(t, 41).comments.at(-1)?.text).toBe(
      "Got it. I added this to the plan for my next turn.",
    );
  });

  it("deletes comments and toggles members", () => {
    t.run("deleteComment", 41, cardOf(t, 41).comments[0]?.id);
    t.run("toggleMember", 41, "godana");
    t.run("toggleMember", 41, "ada");
    t.same();
    expect(cardOf(t, 41).members).toEqual(["blair", "godana"]);
    expect(t.port.S.act[41]?.[0]?.text).toBe("Removed Ada Okafor");
  });
});

describe("filters and saved views", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
    t.run("go", "project", "mobile");
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  const ids = (): number[] => t.port.filtered("mobile").map((c) => c.id);

  it.each([
    ["status", "ready"],
    ["role", "Tester"],
    ["agent", "Codex"],
    ["model", "gpt-5-codex"],
    ["label", "ui"],
    ["package", "packages/api-client"],
    ["package", "No package"],
  ] as const)("filters by %s %s", (k, v) => {
    t.run("addFilter", k, v);
    t.run("addFilter", k, v);
    t.same();
    expect(t.ask("filtered", "mobile").port).toEqual(t.ask("filtered", "mobile").proto);
    expect(t.port.S.savedView.mobile).toBeNull();
  });

  it("combines kinds with and, values of one kind with or", () => {
    t.run("addFilter", "status", "working");
    t.run("addFilter", "status", "review");
    expect(ids()).toEqual([209, 207, 206, 213]);
    t.run("addFilter", "agent", "Codex");
    expect(ids()).toEqual([206, 213]);
    t.run("removeFilter", "agent", "Codex");
    expect(ids()).toEqual([209, 207, 206, 213]);
    t.same();
  });

  it("searches titles, ids, and branches", () => {
    t.port.S.query.mobile = "#21";
    expect(ids()).toEqual([210, 211, 212, 213]);
    t.port.S.query.mobile = "SPLASH";
    expect(ids()).toEqual([206]);
    t.port.S.query.mobile = "marshal/209";
    expect(ids()).toEqual([209]);
    t.run("clearFilters");
    expect(t.port.S.query.mobile).toBe("");
    expect(ids()).toHaveLength(9);
  });

  it("applies and saves views", () => {
    t.run("applyView", "api-client only");
    t.same();
    expect(ids()).toEqual([210, 208]);
    t.run("applyView", "nope");
    t.run("addFilter", "label", "feature");
    t.run("saveView", "Features");
    t.run("saveView", "");
    t.same();
    expect(t.port.S.savedView.mobile).toBe("Features");
    t.run("saveView", "Features");
    t.same();
    expect(t.port.S.savedViews.mobile?.map((v) => v.name)).toEqual([
      "By package",
      "All cards",
      "api-client only",
      "Features",
    ]);
  });
});
