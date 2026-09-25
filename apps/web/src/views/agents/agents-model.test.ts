import { describe, expect, it, vi } from "vitest";
import { type Card, M } from "~/mock";
import {
  activityOf,
  columnsFor,
  sessionIcon,
  sessionLabel,
  sortAgentCards,
  withSessions,
} from "./agents-model";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const card = (patch: Partial<Card>): Card => ({ ...(M.card(41) as Card), ...patch });
const ids = (cards: readonly Card[]): number[] => cards.map((c) => c.id);

describe("sessionLabel and sessionIcon", () => {
  it("say Awake with a sun for a live session", () => {
    expect(sessionLabel(card({}))).toBe("Awake");
    expect(sessionIcon(card({}))).toBe("sun");
  });

  it("say Asleep with a moon, and Waking while a sleeping card wakes", () => {
    expect(sessionLabel(card({ asleep: true }))).toBe("Asleep");
    expect(sessionIcon(card({ asleep: true }))).toBe("moon");
    expect(sessionLabel(card({ asleep: true, waking: true }))).toBe("Waking");
  });

  it("say Stopped with a stop circle for a done card", () => {
    expect(sessionLabel(card({ state: "done" }))).toBe("Stopped");
    expect(sessionIcon(card({ state: "done" }))).toBe("circle-stop");
  });
});

describe("activityOf", () => {
  it("gives the reason for a card that needs you", () => {
    expect(activityOf(card({ state: "needs", reason: "Plan ready for review" }))).toBe(
      "Plan ready for review",
    );
  });

  it("says Paused by you for a paused card", () => {
    expect(activityOf(card({ paused: true, doing: "Running tests" }))).toBe("Paused by you");
  });

  it("gives what the agent is doing, else Merged for done cards and Idle for the rest", () => {
    expect(activityOf(card({ doing: "Running tests" }))).toBe("Running tests");
    expect(activityOf(card({ doing: "", state: "done" }))).toBe("Merged");
    expect(activityOf(card({ doing: "", state: "review" }))).toBe("Idle");
  });
});

describe("sortAgentCards", () => {
  const list = [
    card({ id: 1, state: "review", cost: 2, role: "Worker", think: "Low", doing: "b" }),
    card({ id: 2, state: "working", cost: 1, role: "Tester", think: "High", doing: "a" }),
    card({ id: 3, state: "working", cost: 3, role: "Worker", think: null, doing: "" }),
    card({ id: 4, state: "done", cost: 1, role: "Docs writer", think: "Medium", doing: "c" }),
  ];

  it("orders by state, working first, newest first inside one state", () => {
    expect(ids(sortAgentCards(list, { k: "state", dir: 1 }))).toEqual([3, 2, 1, 4]);
  });

  it("reverses the whole order, including the tie order, for descending", () => {
    expect(ids(sortAgentCards(list, { k: "state", dir: -1 }))).toEqual([4, 1, 2, 3]);
  });

  it("orders numbers by value, ties newest first", () => {
    expect(ids(sortAgentCards(list, { k: "cost", dir: 1 }))).toEqual([4, 2, 1, 3]);
    expect(ids(sortAgentCards(list, { k: "card", dir: 1 }))).toEqual([1, 2, 3, 4]);
    expect(ids(sortAgentCards(list, { k: "card", dir: -1 }))).toEqual([4, 3, 2, 1]);
  });

  it("orders Thinking by effort, with cards that do not think first", () => {
    expect(ids(sortAgentCards(list, { k: "think", dir: 1 }))).toEqual([3, 1, 4, 2]);
  });

  it("orders text by code unit and by activity text", () => {
    expect(ids(sortAgentCards(list, { k: "role", dir: 1 }))).toEqual([4, 2, 3, 1]);
    expect(ids(sortAgentCards(list, { k: "activity", dir: 1 }))).toEqual([3, 2, 1, 4]);
  });

  it("falls back to newest first for an unknown column, and does not change the input", () => {
    const before = ids(list);
    expect(ids(sortAgentCards(list, { k: "nope", dir: 1 }))).toEqual([4, 3, 2, 1]);
    expect(ids(list)).toEqual(before);
  });
});

describe("withSessions", () => {
  it("leaves out backlog cards", () => {
    expect(
      ids(withSessions([card({ id: 1, state: "backlog" }), card({ id: 2, state: "working" })])),
    ).toEqual([2]);
  });
});

describe("columnsFor", () => {
  it("has 11 columns when wide and hides Thinking, Permission mode, and Session otherwise", () => {
    expect(columnsFor(true)).toHaveLength(11);
    expect(columnsFor(false).map((c) => c.label)).toEqual([
      "Card",
      "Role",
      "Agent",
      "Model",
      "State",
      "Current activity",
      "Cost",
      "Actions",
    ]);
  });

  it("gives the Actions column no sort key", () => {
    expect(columnsFor(true).at(-1)?.key).toBe("");
  });
});
