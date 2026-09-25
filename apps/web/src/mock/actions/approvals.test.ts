import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type CardKey, cardNumber } from "../card-key";
import { FIXED_TIME, makeTwin, SLOW_TEST_MS, type Twin } from "../testing/twin";
import type { Msg } from "../types";

const chatOf = (t: Twin, id: CardKey): Msg[] => t.port.S.chat[id] ?? [];
const cardOf = (t: Twin, id: CardKey) => t.port.S.cards.find((c) => c.id === id);

describe("approvals and plans", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it("approves the command on #44 and plays the upgrade", () => {
    expect(t.port.pendingApproval("api#44")?.k).toBe("approval");
    t.run("approve", "api#44");
    t.same();
    expect(cardOf(t, "api#44")?.state).toBe("working");
    const projectApproval = t.port.S.chats.api?.[1]?.msgs.find((m) => m.k === "approval");
    expect(projectApproval?.st).toBe("approved");
    t.play(20_000, 100);
    expect(cardOf(t, "api#44")?.doing).toBe("Committing the upgrade");
    expect(
      chatOf(t, "api#44")
        .filter((m) => m.k === "tool")
        .at(-1),
    ).toMatchObject({
      action: "Ran go test ./...",
      result: "229 passed",
      st: "ok",
    });
    expect(t.port.pendingApproval("api#44")).toBeUndefined();
  });

  it("denies the command on #44 and asks a follow-up question", () => {
    t.run("deny", "api#44");
    t.same();
    t.play(8000, 100);
    expect(cardOf(t, "api#44")).toMatchObject({
      state: "needs",
      reason: "Question: pin to v1.65.1 instead?",
    });
    t.run("deny", "api#44");
    t.same();
  });

  it("approving a card with only a plan approves the plan", () => {
    t.run("approve", "api#43");
    t.same();
    t.play(12_000, 100);
    expect(cardOf(t, "api#43")).toMatchObject({ state: "working", perm: "Auto-accept edits" });
    expect(t.port.S.feed[0]?.text).toBe("You approved the plan on #43");
  });

  it("does nothing when nothing waits", () => {
    t.run("approve", "api#41");
    t.run("approvePlan", "api#41");
    t.run("rejectPlan", "api#41");
    t.run("editPlan", "api#41", true);
    t.run("savePlan", "api#41", "a");
    t.same();
    expect(t.port.S.toasts).toHaveLength(0);
  });

  it("rejects a plan and reworks it", () => {
    t.run("rejectPlan", "api#43");
    t.same();
    t.play(8000, 100);
    expect(cardOf(t, "api#43")).toMatchObject({ state: "planning", doing: "Reworking the plan" });
  });

  it("edits and saves plan steps", () => {
    t.run("editPlan", "api#43", true);
    t.same();
    t.run("editPlan", "api#43", false);
    t.run("savePlan", "api#43", "  First step \n\n Second step  \n");
    t.same();
    const plan = chatOf(t, "api#43").find((m) => m.k === "plan");
    expect(plan).toMatchObject({
      steps: ["First step", "Second step"],
      edited: true,
      editing: false,
    });
    t.run("approvePlan", "api#43");
    t.play(8000, 100);
  });

  it("approves a plan whose steps were all removed", () => {
    t.run("savePlan", "api#43", "   ");
    t.port.approvePlan("api#43");
    expect(cardOf(t, "api#43")?.doing).toBe("");
  });

  it("toggles a tool call's details", () => {
    const tool = chatOf(t, "api#41").find((m) => m.k === "tool");
    t.run("toggleTool", "api#41", tool?.id);
    t.same();
    expect(chatOf(t, "api#41").find((m) => m.id === tool?.id)).toMatchObject({ open: true });
  });

  it("runs the checks, failing the first while CI fails", () => {
    t.run("runChecks", "web#119");
    t.run("runChecks", "api#41");
    t.same();
    t.play(5000, 100);
    expect(t.port.S.checks["web#119"]?.[0]?.st).toBe("failed");
    expect(t.port.S.checks["api#41"]?.every((k) => !k.cmd || k.st === "passed")).toBe(true);
  });

  it.each(["api#40", "mobile#213", "web#118"])("simulates a fast CI failure on %s", (id) => {
    t.run("simulateCiFailure", id);
    t.run("simulateCiFailure", id);
    t.same();
    t.play(15_000, 100);
    expect(cardOf(t, id)?.state).toBe("needs");
    expect(t.port.S.notices[0]).toMatchObject({
      kind: "ci",
      text: `CI failed on #${cardNumber(id)}`,
    });
  });
});
