import { describe, expect, it } from "vitest";
import { type BoardSheetInput, boardSheet, plainCell } from "./board-sheet";

const NAMES: Record<string, string> = { ada: "Ada Okafor", blair: "Blair" };
const DUE = Date.UTC(2026, 9, 20);
const UPDATED = Date.UTC(2026, 9, 6, 23, 59);

type SheetCard = BoardSheetInput["cards"][number];

const card = (fields: Partial<SheetCard> = {}): SheetCard => ({
  n: 41,
  title: "Fix token refresh",
  state: "review",
  role: "Implementer",
  agent: "Claude Code",
  labels: ["bug", "auth"],
  members: ["ada", "blair"],
  due: DUE,
  branch: "marshal/41-fix",
  upd: UPDATED,
  ...fields,
});

const input = (cards: SheetCard[]): BoardSheetInput => ({
  project: "api-gateway",
  cards,
  stateLabel: (state) => (state === "merging" ? "Merging" : "In review"),
  columnLabel: (state) => (state === "merging" ? "Ready to merge" : "In review"),
  nameOf: (id) => NAMES[id] ?? id,
});

describe("boardSheet", () => {
  it("writes a header row, then one row for each card", () => {
    const sheet = boardSheet(input([card(), card({ n: 42, title: "Second", state: "merging" })]));
    expect(sheet.title).toBe("api-gateway board");
    expect(sheet.rows[0]).toEqual([
      "Card",
      "Title",
      "Column",
      "State",
      "Role",
      "Agent",
      "Labels",
      "Members",
      "Due",
      "Branch",
      "Updated",
    ]);
    expect(sheet.rows).toHaveLength(3);
    expect(sheet.rows[1]).toEqual([
      "#41",
      "Fix token refresh",
      "In review",
      "In review",
      "Implementer",
      "Claude Code",
      "bug, auth",
      "Ada Okafor, Blair",
      "2026-10-20",
      "marshal/41-fix",
      "2026-10-06",
    ]);
    expect(sheet.rows[2]?.slice(2, 4)).toEqual(["Ready to merge", "Merging"]);
  });

  it("is the header alone for an empty board, and leaves empty fields empty", () => {
    expect(boardSheet(input([])).rows).toHaveLength(1);
    const bare = boardSheet(input([card({ labels: [], members: [], due: null, branch: null })]));
    expect(bare.rows[1]?.slice(6)).toEqual(["", "", "", "", "2026-10-06"]);
  });

  it("puts a quote in front of a title a sheet would run as a formula", () => {
    const risky = ['=HYPERLINK("http://evil","x")', "+1+1", "@SUM(A1)", "-cmd|' /C calc'!A0"];
    const rows = boardSheet(input(risky.map((title) => card({ title })))).rows.slice(1);
    expect(rows.map((row) => row[1])).toEqual(risky.map((title) => `'${title}`));
  });

  it("leaves a negative number, markup and ordinary text as they are", () => {
    expect(plainCell("-5")).toBe("-5");
    expect(plainCell("-2.50")).toBe("-2.50");
    expect(plainCell("a < b & c = d")).toBe("a < b & c = d");
    expect(plainCell("'=already quoted")).toBe("'=already quoted");
    expect(plainCell("Fix =login")).toBe("Fix =login");
  });

  it("keeps a title on one line", () => {
    const rows = boardSheet(input([card({ title: "One\nTwo\tthree" })])).rows;
    expect(rows[1]?.[1]).toBe("One Two three");
  });
});
