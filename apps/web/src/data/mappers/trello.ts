import type {
  ApplyTrelloResult,
  TrelloChange,
  TrelloChangeKind,
  TrelloLinkedCard,
  TrelloPending,
} from "@marshal/protocol";
import { toMillis } from "./time";

/*
 * A project's Trello changes in the words the board and the Apply dialog draw (docs/marshal-product-scope.md
 * section 19.2). Marshal works on its own copy of a linked Trello board. What it changed since it
 * last read Trello is a list of pending changes, and nothing reaches Trello until the person picks
 * some of them. Who made a change (a person or the agent) is shown only inside Marshal, because
 * Trello never shows it: everything is written as the linked person.
 */

/** Who made a change in Marshal, or empty when that is not known. */
export type ChangeAuthor = "person" | "agent" | "";

/** One thing Marshal would do in Trello. */
export interface TrelloChangeView {
  /** The change's id. It changes when the change does, so a dismissed change that is edited returns. */
  id: string;
  /** The daemon's id for the Marshal card the change belongs to. */
  cardId: string;
  cardTitle: string;
  kind: TrelloChangeKind;
  /** One line that says what the change does, such as "Move to Done". */
  summary: string;
  /** The text that would be written, such as a comment's body. Empty when the summary says it all. */
  detail: string;
  by: ChangeAuthor;
  /** "by you", "by the agent", or empty when who made it is not known. */
  byLabel: string;
  /** Trello changed the same thing since Marshal last read it. */
  conflict: boolean;
  /** What changed in Trello, such as "Trello moved this card to Review." Empty without a conflict. */
  conflictNote: string;
}

/** The pending changes of one Marshal card, in the order they would be applied. */
export interface TrelloCardGroup {
  cardId: string;
  cardTitle: string;
  changes: TrelloChangeView[];
}

/** A project's Trello state as the store keeps it. */
export interface TrelloView {
  /** False for a project with no Trello board: everything below is then empty. */
  linked: boolean;
  boardName: string;
  changes: TrelloChangeView[];
  /** The project's cards that have a Trello card. */
  cards: TrelloLinkedCard[];
  /** When Marshal last read the board, in ms, or null if it never did. */
  fetchedAt: number | null;
  /** The daemon's clock when it answered, in ms. */
  serverTime: number;
}

/**
 * What the store keeps for one project: the state once it was read, the sentence for a read that
 * failed, and whether a first read is still on its way.
 */
export interface TrelloSlot {
  pending: TrelloView | null;
  error: string;
  loading: boolean;
}

/** One change in the report that follows an Apply. */
export interface TrelloReportRow {
  id: string;
  cardTitle: string;
  summary: string;
}

/** A change that was not written, with the daemon's sentence for why. */
export interface TrelloSkippedRow extends TrelloReportRow {
  reason: string;
}

/** What an Apply wrote and what it left, drawn once it is done. */
export interface TrelloApplyReport {
  applied: TrelloReportRow[];
  skipped: TrelloSkippedRow[];
}

/** "by you", "by the agent", or empty when who made the change is not known. */
function authorLabel(by: ChangeAuthor): string {
  if (by === "person") return "by you";
  return by === "agent" ? "by the agent" : "";
}

/** The author a wire value names. Anything but "person" or "agent" is not known. */
function authorOf(value: string): ChangeAuthor {
  return value === "person" || value === "agent" ? value : "";
}

function changeView(change: TrelloChange): TrelloChangeView {
  const by = authorOf(change.byKind);
  return {
    id: change.id,
    cardId: change.cardId,
    cardTitle: change.cardTitle,
    kind: change.kind,
    summary: change.summary,
    detail: change.detail,
    by,
    byLabel: authorLabel(by),
    conflict: change.conflict,
    conflictNote: change.conflictNote,
  };
}

/** A project's Trello state from the daemon's answer. */
export function toTrelloView(wire: TrelloPending): TrelloView {
  return {
    linked: wire.linked,
    boardName: wire.boardName,
    changes: wire.changes.map(changeView),
    cards: wire.cards,
    fetchedAt: wire.fetchedAt ? toMillis(wire.fetchedAt) : null,
    serverTime: toMillis(wire.serverTime),
  };
}

/** Groups changes by the card they belong to, keeping the daemon's order of cards and changes. */
export function groupByCard(changes: readonly TrelloChangeView[]): TrelloCardGroup[] {
  const groups = new Map<string, TrelloCardGroup>();
  for (const change of changes) {
    const group = groups.get(change.cardId);
    if (group) group.changes.push(change);
    else
      groups.set(change.cardId, {
        cardId: change.cardId,
        cardTitle: change.cardTitle,
        changes: [change],
      });
  }
  return [...groups.values()];
}

/** The Trello card a Marshal card has, by the daemon's card id, or undefined. */
export function linkOf(
  view: TrelloView | null | undefined,
  cardId: string | undefined,
): TrelloLinkedCard | undefined {
  if (!(view && cardId)) return undefined;
  return view.cards.find((link) => link.cardId === cardId);
}

/**
 * The Trello card of a card in the store. The daemon names a card by its own id; a store with no
 * daemon names it by its key, so either one is looked for.
 */
export const linkOfCard = (
  view: TrelloView | null | undefined,
  card: { id: string; daemonId?: string | undefined },
): TrelloLinkedCard | undefined => linkOf(view, card.daemonId) ?? linkOf(view, card.id);

/** The words for where a linked card's text came from. */
export const markerWord = (link: TrelloLinkedCard): string =>
  link.fromTrello ? "From Trello" : "On Trello";

/** The changes that are not in conflict, which the dialog starts with ticked. */
export const calmChanges = (changes: readonly TrelloChangeView[]): TrelloChangeView[] =>
  changes.filter((change) => !change.conflict);

/**
 * What an Apply did, in the words of the changes the person saw. The answer's own pending list no
 * longer holds a change that was written, so the list from before is what names them.
 */
export function toApplyReport(
  before: readonly TrelloChangeView[],
  result: Pick<ApplyTrelloResult, "applied" | "skipped">,
): TrelloApplyReport {
  const byId = new Map(before.map((change) => [change.id, change]));
  const row = (id: string): TrelloReportRow => {
    const change = byId.get(id);
    return { id, cardTitle: change?.cardTitle ?? "", summary: change?.summary ?? "A change" };
  };
  return {
    applied: result.applied.map(row),
    skipped: result.skipped.map((skip) => ({ ...row(skip.changeId), reason: skip.reason })),
  };
}

/** How many changes would be written, in the words of the dialog's button. */
export function applyLabel(count: number): string {
  return count === 1 ? "Apply 1 change" : `Apply ${count} changes`;
}
