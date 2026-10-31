import { describe, expect, it } from "vitest";
import type {
  ApplyTrelloRequest,
  ApplyTrelloResult,
  DismissTrelloRequest,
  RefreshTrelloRequest,
  RefreshTrelloResult,
  TrelloChange,
  TrelloLinkedCard,
  TrelloPending,
} from "../src";
import { EventTypeTrelloChanged, TrelloChangeKindValues } from "../src";
import { golden } from "./golden";

// The Trello apply samples, checked against the generated types by the compiler and against the Go
// goldens at run time, the same way the other golden files are.

const CARD_ID = "01M3C107JB041061050R3GG28A";
const SERVER_TIME = "2026-09-27T09:30:00.000Z";

const move: TrelloChange = {
  id: "chg-move-1",
  cardId: CARD_ID,
  cardTitle: "Fix the report export",
  kind: "moveDone",
  summary: "Move to Done",
  detail: "",
  byKind: "person",
  conflict: true,
  conflictNote: "Trello moved this card to Review.",
};

const comment: TrelloChange = {
  id: "chg-comment-1",
  cardId: CARD_ID,
  cardTitle: "Fix the report export",
  kind: "addComment",
  summary: "Add a comment",
  detail: "The export now streams rows, so large reports no longer time out.",
  byKind: "agent",
  conflict: false,
  conflictNote: "",
};

const linked: TrelloLinkedCard = {
  cardId: CARD_ID,
  url: "https://trello.com/c/Ab12CdEf",
  listName: "In review",
  fromTrello: true,
};

const pending: TrelloPending = {
  linked: true,
  boardName: "Mobile app",
  changes: [move, comment],
  cards: [linked],
  fetchedAt: "2026-09-27T09:26:00.000Z",
  serverTime: SERVER_TIME,
};

describe("Trello pending golden files", () => {
  it("has a linked project with a conflict, a comment, and the Trello card", () => {
    expect(golden("trello-pending")).toEqual(pending);
  });

  it("has a project with no Trello board as empty lists and no read time", () => {
    const sample: TrelloPending = {
      linked: false,
      boardName: "",
      changes: [],
      cards: [],
      fetchedAt: null,
      serverTime: SERVER_TIME,
    };
    expect(golden("trello-pending-unlinked")).toEqual(sample);
  });

  it("names only kinds the generated list knows", () => {
    const sample = golden("trello-pending") as TrelloPending;
    for (const change of sample.changes) expect(TrelloChangeKindValues).toContain(change.kind);
  });

  it("has the event that tells the app to read the changes again", () => {
    expect(EventTypeTrelloChanged).toBe("trello.changed");
  });
});

describe("Trello apply golden files", () => {
  it("has the apply request, with the override inside the changes it lists", () => {
    const sample: ApplyTrelloRequest = {
      changeIds: ["chg-move-1", "chg-comment-1"],
      overrideIds: ["chg-move-1"],
    };
    expect(golden("trello-apply-request")).toEqual(sample);
  });

  it("has the apply answer: what was written, what was skipped and why, and what is left", () => {
    const sample: ApplyTrelloResult = {
      applied: ["chg-comment-1"],
      skipped: [{ changeId: "chg-move-1", reason: "The Trello card was deleted." }],
      pending: {
        linked: true,
        boardName: "Mobile app",
        changes: [],
        cards: [],
        fetchedAt: null,
        serverTime: SERVER_TIME,
      },
    };
    expect(golden("trello-apply-result")).toEqual(sample);
  });

  it("has the dismiss request", () => {
    const sample: DismissTrelloRequest = { changeIds: ["chg-move-1"] };
    expect(golden("trello-dismiss-request")).toEqual(sample);
  });
});

describe("Trello refresh golden files", () => {
  it("has the refresh request, naming the cards to set back to Trello's version", () => {
    const sample: RefreshTrelloRequest = { keepTrelloCardIds: [CARD_ID] };
    expect(golden("trello-refresh-request")).toEqual(sample);
  });

  it("has the refresh answer: the counts, and what is pending afterwards", () => {
    const sample: RefreshTrelloResult = { imported: 3, updated: 1, pending };
    expect(golden("trello-refresh-result")).toEqual(sample);
  });
});
