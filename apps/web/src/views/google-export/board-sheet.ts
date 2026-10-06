import type { CreateGoogleSheetRequest } from "@marshal/protocol";
import type { Card, Status } from "~/mock";
import { clip, dayOf, MAX_TITLE_CHARS, oneLine } from "./export-text";

type SheetCard = Pick<
  Card,
  "n" | "title" | "state" | "role" | "agent" | "labels" | "members" | "due" | "branch" | "upd"
>;

export interface BoardSheetInput {
  project: string;
  /** The project's cards, in the order the sheet should list them. */
  cards: readonly SheetCard[];
  /** The name of a card's state, and of the board column it sits in. */
  stateLabel: (state: Status) => string;
  columnLabel: (state: Status) => string;
  /** The name of the person with this id. */
  nameOf: (id: string) => string;
}

const HEADER = [
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
];

const FORMULA_START = /^[=+@\t\r]/;
const PLAIN_NUMBER = /^-\d+(\.\d+)?$/;

/**
 * A cell that Sheets would run as a formula gets a quote in front, so it stays the text it is. The
 * daemon does the same, and leaves a cell that starts with a quote alone.
 */
export function plainCell(text: string): string {
  const risky = FORMULA_START.test(text) || (text.startsWith("-") && !PLAIN_NUMBER.test(text));
  return risky ? `'${text}` : text;
}

function rowOf(card: SheetCard, input: BoardSheetInput): string[] {
  return [
    `#${card.n}`,
    oneLine(card.title),
    input.columnLabel(card.state),
    input.stateLabel(card.state),
    card.role,
    card.agent,
    card.labels.join(", "),
    card.members.map((id) => input.nameOf(id)).join(", "),
    dayOf(card.due),
    card.branch ?? "",
    dayOf(card.upd),
  ].map(plainCell);
}

/** A Google Sheet of a board: a header row, then one row for each card. */
export function boardSheet(input: BoardSheetInput): CreateGoogleSheetRequest {
  const title = clip(oneLine(`${input.project} board`), MAX_TITLE_CHARS);
  return { title, rows: [HEADER, ...input.cards.map((card) => rowOf(card, input))] };
}
