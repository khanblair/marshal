import type { IntegrationState } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "~/data/testing/golden";
import { isPaused, resolvedLabel, toMergeFlow } from "./integration";

const wire = golden<IntegrationState>("integration-state");
const idle: IntegrationState = {
  ...wire,
  state: "idle",
  currentCardId: "",
  queue: [],
  history: [],
};

describe("toMergeFlow", () => {
  it("groups the queue into the lanes a card passes through, in the daemon's order", () => {
    const flow = toMergeFlow(wire);
    expect(flow.lanes.resolving.map((card) => card.key)).toEqual(["web#12"]);
    expect(flow.lanes.queued.map((card) => card.key)).toEqual(["web#13"]);
    expect(flow.lanes.testing).toEqual([]);
    expect(flow.lanes.landing).toEqual([]);
    expect(flow.queued).toBe(2);
    expect(flow.lanes.resolving[0]).toEqual({
      cardId: "01HZ",
      key: "web#12",
      title: "Add login page",
      phase: "resolving",
      position: 1,
    });
  });

  it("carries the branches, how far ahead the Integrator is, and the daemon's clock", () => {
    const flow = toMergeFlow(wire);
    expect(flow).toMatchObject({
      projectId: "web",
      target: "development",
      integratorBranch: "integrator",
      aheadBy: 2,
      currentCardId: "01HZ",
      message: "",
    });
    expect(flow.serverTime).toBe(Date.parse("2026-09-27T09:30:00.000Z"));
  });

  it("says each state in plain words and names the card being merged", () => {
    expect(toMergeFlow(wire).stateLabel).toBe("Merging web#12");
    expect(toMergeFlow({ ...wire, currentCardId: "gone" }).stateLabel).toBe("Merging");
    expect(toMergeFlow(idle).stateLabel).toBe("Idle");
    expect(
      toMergeFlow({ ...idle, state: "waiting", message: "A conflict needs you." }),
    ).toMatchObject({
      stateLabel: "Waiting for you",
      message: "A conflict needs you.",
    });
    const paused = toMergeFlow({ ...idle, state: "paused" });
    expect(paused.stateLabel).toBe("Paused");
    expect(isPaused(paused)).toBe(true);
    expect(isPaused(toMergeFlow(wire))).toBe(false);
  });

  it("reads the delivered cards with their time, a short commit, and the conflicts resolved", () => {
    const [delivered] = toMergeFlow(wire).delivered;
    expect(delivered).toEqual({
      cardId: "01GX",
      key: "web#11",
      title: "Rename config",
      at: Date.parse("2026-09-27T09:30:00.000Z"),
      commit: "3f2a9c1",
      resolved: 1,
      resolvedLabel: "Resolved 1 conflict",
      summary: "Both cards edited config.py; kept both settings.",
      canUndo: true,
    });
  });

  it("gives an idle project empty lanes and no deliveries", () => {
    const flow = toMergeFlow(idle);
    expect(flow.queued).toBe(0);
    expect(Object.values(flow.lanes).every((lane) => lane.length === 0)).toBe(true);
    expect(flow.delivered).toEqual([]);
  });

  it("ignores a phase it does not know instead of failing the whole read", () => {
    const odd = {
      ...wire,
      queue: [{ ...wire.queue[0]!, phase: "polishing" as never }, wire.queue[1]!],
    };
    const flow = toMergeFlow(odd);
    expect(flow.lanes.queued.map((card) => card.key)).toEqual(["web#13"]);
    expect(flow.queued).toBe(1);
  });
});

describe("resolvedLabel", () => {
  it("says nothing for a clean merge, and counts the conflicts otherwise", () => {
    expect(resolvedLabel(0)).toBe("");
    expect(resolvedLabel(1)).toBe("Resolved 1 conflict");
    expect(resolvedLabel(3)).toBe("Resolved 3 conflicts");
  });
});
