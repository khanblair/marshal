import { describe, expect, it, vi } from "vitest";
import { type Card, M } from "~/mock";
import { noMatchText, sortListCards, visibleColumns } from "./list-columns";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const base = M.card("api#41") as Card;
const card = (patch: Partial<Card>): Card => ({ ...base, ...patch });
/** The numbers of the cards, which is what the sort orders show as `#41`. */
const ids = (cards: readonly Card[]): number[] => cards.map((c) => c.n);
const DEFAULT_COLS = { ...M.S.listCols };
const DESKTOP_PX = 1440;
const TABLET_PX = 820;

describe("visibleColumns", () => {
  const keys = (cols: Record<string, boolean>, vw: number): string[] =>
    visibleColumns(cols, vw).map((c) => c.key);

  it("shows the default columns at desktop width, in display order", () => {
    expect(keys(DEFAULT_COLS, DESKTOP_PX)).toEqual([
      "id",
      "title",
      "state",
      "role",
      "agent",
      "model",
      "branch",
      "ci",
      "cost",
      "upd",
    ]);
  });

  it("adds Thinking and Package where the menu turned them on", () => {
    expect(keys({ ...DEFAULT_COLS, think: true, pkg: true }, DESKTOP_PX)).toEqual([
      "id",
      "title",
      "state",
      "role",
      "agent",
      "model",
      "think",
      "pkg",
      "branch",
      "ci",
      "cost",
      "upd",
    ]);
  });

  it("hides Model, Thinking, Package, and Updated below 1200 px", () => {
    expect(keys({ ...DEFAULT_COLS, think: true, pkg: true }, TABLET_PX)).toEqual([
      "id",
      "title",
      "state",
      "role",
      "agent",
      "branch",
      "ci",
      "cost",
    ]);
  });

  it("leaves out columns the menu turned off", () => {
    expect(keys({ id: true, title: true }, DESKTOP_PX)).toEqual(["id", "title"]);
    expect(keys({}, DESKTOP_PX)).toEqual([]);
  });

  it("right-aligns only Cost", () => {
    const aligned = visibleColumns(DEFAULT_COLS, DESKTOP_PX).filter((c) => c.alignEnd);
    expect(aligned.map((c) => c.label)).toEqual(["Cost"]);
  });
});

describe("sortListCards", () => {
  const list = [
    card({
      id: "api#1",
      n: 1,
      title: "beta",
      state: "review",
      cost: 2,
      ci: "passed",
      pkg: "b",
      think: "Low",
    }),
    card({
      id: "api#2",
      n: 2,
      title: "Alpha",
      state: "working",
      cost: 1,
      ci: null,
      pkg: null,
      think: null,
    }),
    card({
      id: "api#3",
      n: 3,
      title: "gamma",
      state: "merging",
      cost: 3,
      ci: "failed",
      pkg: "a",
      think: "High",
    }),
    card({
      id: "api#4",
      n: 4,
      title: "delta",
      state: "backlog",
      cost: 1,
      ci: "running",
      pkg: "a",
      think: "Low",
    }),
  ];
  const sorted = (k: string, dir: number): number[] => ids(sortListCards(list, { k, dir }));

  it("sorts the title without regard to case", () => {
    expect(sorted("title", 1)).toEqual([2, 1, 4, 3]);
    expect(sorted("title", -1)).toEqual([3, 4, 1, 2]);
  });

  it("sorts state by board column, with merging in Ready to merge", () => {
    expect(sorted("state", 1)).toEqual([4, 2, 1, 3]);
  });

  it("sorts numbers by value and keeps the incoming order for ties", () => {
    expect(sorted("cost", 1)).toEqual([2, 4, 1, 3]);
    expect(sorted("cost", -1)).toEqual([3, 1, 2, 4]);
    expect(sorted("id", -1)).toEqual([4, 3, 2, 1]);
  });

  it("sorts missing values as empty text, before any text", () => {
    expect(sorted("ci", 1)).toEqual([2, 3, 1, 4]);
    expect(sorted("pkg", 1)).toEqual([2, 3, 4, 1]);
    expect(sorted("think", 1)).toEqual([2, 3, 1, 4]);
  });

  it("sorts every other key without failing", () => {
    for (const k of ["role", "agent", "model", "branch", "upd"]) {
      expect(sorted(k, 1)).toHaveLength(4);
    }
  });

  it("falls back to the id for an unknown key, and does not change the input", () => {
    const before = ids(list);
    expect(sorted("nope", 1)).toEqual([1, 2, 3, 4]);
    expect(ids(list)).toEqual(before);
  });
});

describe("noMatchText", () => {
  it("quotes the search text, or names the filters", () => {
    expect(noMatchText("auth")).toBe('No cards match "auth".');
    expect(noMatchText("")).toBe("No cards match these filters.");
    expect(noMatchText(undefined)).toBe("No cards match these filters.");
  });
});
