import type { TrelloApplySkip, TrelloChangeKind } from "@marshal/protocol";
import type { TrelloChangeView, TrelloView } from "~/data/mappers/trello";

/*
 * Trello with no daemon. A store with no daemon (a test, or the mock) has no board to read, so the
 * Apply flow works on the state it already holds: the same four steps the daemon has (apply,
 * dismiss, refresh), on a `TrelloView`. `mockTrello` is a linked project with a few pending
 * changes, one of them in conflict, on cards the prototype's own board has.
 */

const CONFLICT_SKIPPED = "Trello changed this after Marshal last read it, so it was left alone.";

/** The kinds a refresh sets back to Trello's version for a card: its title, description, and ticks. */
const RESET_KINDS: ReadonlySet<TrelloChangeKind> = new Set(["updateCard", "setItem"]);

const MINUTE_MS = 60_000;
/** How many minutes before now the mock's board was last read. */
const READ_AGE_MINUTES = 4;

/** A linked project with five pending changes: a move in conflict, comments, a tick, and a rewrite. */
export function mockTrello(now: number = Date.now()): TrelloView {
  return {
    linked: true,
    boardName: "Web dashboard",
    changes: [
      {
        id: "mock-move-118",
        cardId: "web#118",
        cardTitle: "Dark mode for settings page",
        kind: "moveDone",
        summary: "Move to Done",
        detail: "",
        by: "person",
        byLabel: "by you",
        conflict: true,
        conflictNote: "Trello moved this card to Review.",
      },
      {
        id: "mock-comment-118",
        cardId: "web#118",
        cardTitle: "Dark mode for settings page",
        kind: "addComment",
        summary: "Add a comment",
        detail:
          "Theme tokens now follow the system setting, and the settings page keeps its contrast.",
        by: "agent",
        byLabel: "by the agent",
        conflict: false,
        conflictNote: "",
      },
      {
        id: "mock-tick-118",
        cardId: "web#118",
        cardTitle: "Dark mode for settings page",
        kind: "setItem",
        summary: "Tick Check contrast",
        detail: "",
        by: "agent",
        byLabel: "by the agent",
        conflict: false,
        conflictNote: "",
      },
      {
        id: "mock-update-119",
        cardId: "web#119",
        cardTitle: "Migrate tables to TanStack Table v8",
        kind: "updateCard",
        summary: "Change the description",
        detail: "Move every table to TanStack Table v8 and keep the sort and filter behavior.",
        by: "person",
        byLabel: "by you",
        conflict: false,
        conflictNote: "",
      },
      {
        id: "mock-create-120",
        cardId: "web#120",
        cardTitle: "Chart tooltips cut off on small screens",
        kind: "createCard",
        summary: "Create this card in Trello",
        detail: "",
        by: "",
        byLabel: "",
        conflict: false,
        conflictNote: "",
      },
    ],
    cards: [
      {
        cardId: "web#118",
        url: "https://trello.com/c/Ab12CdEf",
        listName: "In review",
        fromTrello: true,
      },
      {
        cardId: "web#119",
        url: "https://trello.com/c/Gh34IjKl",
        listName: "Backlog",
        fromTrello: false,
      },
    ],
    fetchedAt: now - READ_AGE_MINUTES * MINUTE_MS,
    serverTime: now,
  };
}

/** What an apply did: the changes it wrote, those it left and why, and what is still pending. */
export interface MockApply {
  applied: string[];
  skipped: TrelloApplySkip[];
  next: TrelloView;
}

/** Writes the chosen changes. A change in conflict is written only when it is also overridden. */
export function mockApply(
  view: TrelloView,
  changeIds: readonly string[],
  overrideIds: readonly string[],
): MockApply {
  const applied: string[] = [];
  const skipped: TrelloApplySkip[] = [];
  for (const id of changeIds) {
    const change = view.changes.find((one) => one.id === id);
    if (!change) skipped.push({ changeId: id, reason: "This change is no longer pending." });
    else if (change.conflict && !overrideIds.includes(id)) {
      skipped.push({ changeId: id, reason: CONFLICT_SKIPPED });
    } else applied.push(id);
  }
  return {
    applied,
    skipped,
    next: { ...view, changes: view.changes.filter((one) => !applied.includes(one.id)) },
  };
}

/** Stops showing the chosen changes. They are never written. */
export function mockDismiss(view: TrelloView, changeIds: readonly string[]): TrelloView {
  return { ...view, changes: view.changes.filter((one) => !changeIds.includes(one.id)) };
}

/** Reads the board again: the kept cards take Trello's title, description, and ticks. */
export function mockRefresh(view: TrelloView, keepCardIds: readonly string[]): TrelloView {
  const dropped = (change: TrelloChangeView): boolean =>
    keepCardIds.includes(change.cardId) && RESET_KINDS.has(change.kind);
  return { ...view, changes: view.changes.filter((change) => !dropped(change)) };
}
