import type { ApplyTrelloResult, TrelloPending } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "../testing/golden";
import { applyLabel, groupByCard, linkOf, markerWord, toApplyReport, toTrelloView } from "./trello";

const wire = golden<TrelloPending>("trello-pending");
const unlinked = golden<TrelloPending>("trello-pending-unlinked");
const result = golden<ApplyTrelloResult>("trello-apply-result");

describe("toTrelloView", () => {
  it("says who made each change inside Marshal, and carries the conflict and its note", () => {
    const view = toTrelloView(wire);
    expect(view.changes.map((c) => [c.id, c.by, c.byLabel, c.conflict])).toEqual([
      ["chg-move-1", "person", "by you", true],
      ["chg-comment-1", "agent", "by the agent", false],
    ]);
    expect(view.changes[0]?.conflictNote).toBe("Trello moved this card to Review.");
    expect(view.changes[1]?.detail).toContain("streams rows");
  });

  it("says nothing about the author when the daemon does not know it", () => {
    const first = wire.changes[0];
    if (!first) throw new Error("the golden has a change");
    const view = toTrelloView({ ...wire, changes: [{ ...first, byKind: "" }] });
    expect(view.changes[0]).toMatchObject({ by: "", byLabel: "" });
  });

  it("reads the board's time and the daemon's clock in ms, and null when it never read", () => {
    expect(toTrelloView(wire).fetchedAt).toBe(Date.parse("2026-09-27T09:26:00.000Z"));
    expect(toTrelloView(wire).serverTime).toBe(Date.parse("2026-09-27T09:30:00.000Z"));
    expect(toTrelloView(unlinked)).toMatchObject({ linked: false, changes: [], fetchedAt: null });
  });
});

describe("groupByCard", () => {
  it("groups the changes of one card in the daemon's order", () => {
    const groups = groupByCard(toTrelloView(wire).changes);
    expect(groups).toHaveLength(1);
    expect(groups[0]?.cardTitle).toBe("Fix the report export");
    expect(groups[0]?.changes.map((c) => c.id)).toEqual(["chg-move-1", "chg-comment-1"]);
  });
});

describe("the Trello card of a Marshal card", () => {
  it("finds the link by the daemon's card id, and says where the text came from", () => {
    const view = toTrelloView(wire);
    const link = linkOf(view, "01M3C107JB041061050R3GG28A");
    expect(link?.listName).toBe("In review");
    expect(link && markerWord(link)).toBe("From Trello");
    expect(link && markerWord({ ...link, fromTrello: false })).toBe("On Trello");
    expect(linkOf(view, "other")).toBeUndefined();
    expect(linkOf(null, "x")).toBeUndefined();
    expect(linkOf(view, undefined)).toBeUndefined();
  });
});

describe("toApplyReport", () => {
  it("names what was written and what was left, from the changes the person saw", () => {
    const report = toApplyReport(toTrelloView(wire).changes, result);
    expect(report.applied).toEqual([
      { id: "chg-comment-1", cardTitle: "Fix the report export", summary: "Add a comment" },
    ]);
    expect(report.skipped).toEqual([
      {
        id: "chg-move-1",
        cardTitle: "Fix the report export",
        summary: "Move to Done",
        reason: "The Trello card was deleted.",
      },
    ]);
  });

  it("still answers for a change it was never shown", () => {
    expect(toApplyReport([], result).applied[0]).toEqual({
      id: "chg-comment-1",
      cardTitle: "",
      summary: "A change",
    });
  });
});

describe("applyLabel", () => {
  it("counts the changes in the button's words", () => {
    expect(applyLabel(1)).toBe("Apply 1 change");
    expect(applyLabel(3)).toBe("Apply 3 changes");
  });
});
