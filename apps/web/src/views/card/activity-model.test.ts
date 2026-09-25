import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { activityRows, checkpointsFor } from "./activity-model";
import { resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

describe("activityRows", () => {
  beforeEach(() => resetStore());

  it("keeps only the newest 120 entries", () => {
    const many = Array.from({ length: 150 }, (_, i) => ({
      id: `a${i}`,
      kind: "tool" as const,
      text: `Step ${i}`,
      result: "",
      st: "ok" as const,
      ts: Date.now(),
    }));
    expect(activityRows(many)).toHaveLength(120);
  });

  it("sets commands and file verbs in the mono font", () => {
    const rows = activityRows([
      { id: "1", kind: "command", text: "go test", result: "", st: "ok", ts: Date.now() },
      { id: "2", kind: "file", text: "Edited a.go", result: "", st: "ok", ts: Date.now() },
      { id: "3", kind: "tool", text: "You commented", result: "", st: "ok", ts: Date.now() },
    ]);
    expect(rows.map((row) => row.mono)).toEqual([true, true, false]);
    expect(rows.map((row) => row.icon)).toEqual(["terminal", "file-pen", "wrench"]);
  });

  it("colors the result by state", () => {
    const at = Date.now();
    const rows = activityRows(
      (["ok", "fail", "waiting", "running"] as const).map((st, i) => ({
        id: String(i),
        kind: "test" as const,
        text: "Check",
        result: st,
        st,
        ts: at,
      })),
    );
    expect(rows.map((row) => row.resultIcon)).toEqual(["check", "x", "st-needs", "spinner"]);
    expect(rows.map((row) => row.resultClass)).toEqual([
      "text-status-working-text",
      "text-status-danger-text",
      "text-status-needs-you-text",
      "text-secondary",
    ]);
  });

  it("highlights an entry for a moment after it arrives", () => {
    const now = M.now();
    const [fresh, old] = activityRows([
      { id: "1", kind: "tool", text: "New", result: "", st: "ok", ts: now - 200 },
      { id: "2", kind: "tool", text: "Old", result: "", st: "ok", ts: now - 60_000 },
    ]);
    expect([fresh?.fresh, old?.fresh]).toEqual([true, false]);
  });
});

describe("checkpointsFor", () => {
  beforeEach(() => resetStore());

  it("has none before the card starts", () => {
    expect(checkpointsFor(45, false, "a/b.go")).toEqual([]);
  });

  it("has three, named for the turn, the first file, and the session start", () => {
    const list = checkpointsFor(41, true, "internal/proxy/handler.go");
    expect(list.map((item) => item.label)).toEqual([
      "Before turn 6",
      "Before editing handler.go",
      "Session start",
    ]);
    expect(list.map((item) => item.ref)).toEqual([
      "refs/marshal/cp/41/3",
      "refs/marshal/cp/41/2",
      "refs/marshal/cp/41/1",
    ]);
    expect(list.map((item) => item.when)).toEqual(["12 min ago", "38 min ago", "95 min ago"]);
  });

  it("asks before it restores, and says so afterwards", () => {
    checkpointsFor(41, true, "a/b.go")[0]?.restore();
    expect(M.S.dialog?.title).toBe("Restore checkpoint");
    expect(M.S.dialog?.message).toContain('"Before turn 6"');
    M.S.dialog?.run();
    expect(M.S.toasts.at(-1)?.msg).toBe("Checkpoint restored");
  });
});
