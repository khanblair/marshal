import { breakpoints } from "@marshal/tokens/tokens";
import { compareSortValues, type TableSort } from "@marshal/ui";
import { type Card, M } from "~/mock";

/** From this window width the wide columns show. */
const DESKTOP_MIN_WIDTH_PX = breakpoints.lg;

/** Each shown column adds this much to the table's minimum width. */
export const MIN_COLUMN_WIDTH_PX = 110;

export type ListKey =
  | "id"
  | "title"
  | "state"
  | "role"
  | "agent"
  | "model"
  | "think"
  | "pkg"
  | "branch"
  | "ci"
  | "cost"
  | "upd";

export interface ListColumn {
  key: ListKey;
  label: string;
  /** Right-align the header and the cells (numbers). */
  alignEnd?: boolean;
  /** Hidden below the desktop width. */
  wideOnly?: boolean;
}

/** In display order. The Columns menu turns each one on or off through `M.S.listCols`. */
const LIST_COLUMNS: readonly ListColumn[] = [
  { key: "id", label: "ID" },
  { key: "title", label: "Title" },
  { key: "state", label: "State" },
  { key: "role", label: "Role" },
  { key: "agent", label: "Agent" },
  { key: "model", label: "Model", wideOnly: true },
  { key: "think", label: "Thinking", wideOnly: true },
  { key: "pkg", label: "Package", wideOnly: true },
  { key: "branch", label: "Branch" },
  { key: "ci", label: "CI" },
  { key: "cost", label: "Cost", alignEnd: true },
  { key: "upd", label: "Updated", wideOnly: true },
];

/** The columns turned on in the Columns menu that also fit this width. */
export function visibleColumns(listCols: Record<string, boolean>, vw: number): ListColumn[] {
  return LIST_COLUMNS.filter(
    (column) => listCols[column.key] && (vw >= DESKTOP_MIN_WIDTH_PX || !column.wideOnly),
  );
}

type SortValue = string | number;

const SORT_VALUES: Record<ListKey, (card: Card) => SortValue> = {
  id: (card) => card.id,
  title: (card) => card.title.toLowerCase(),
  state: (card) => M.COLUMNS.indexOf(M.colOf(card.state)),
  role: (card) => card.role,
  agent: (card) => card.agent,
  model: (card) => card.model,
  think: (card) => card.think || "",
  pkg: (card) => card.pkg || "",
  branch: (card) => card.branch || "",
  ci: (card) => card.ci || "",
  cost: (card) => card.cost,
  upd: (card) => card.upd,
};

const isListKey = (key: string): key is ListKey => Object.hasOwn(SORT_VALUES, key);

/** Sorts a copy of the cards. Equal cards keep the order they came in. */
export function sortListCards(cards: readonly Card[], sort: TableSort): Card[] {
  const sortValue = SORT_VALUES[isListKey(sort.k) ? sort.k : "id"];
  return cards.slice().sort((a, b) => compareSortValues(sortValue(a), sortValue(b)) * sort.dir);
}

/** What the header says when the filters or search hide every card. */
export function noMatchText(query: string | undefined): string {
  return query ? `No cards match "${query}".` : "No cards match these filters.";
}
