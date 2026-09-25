import { breakpoints } from "@marshal/tokens/tokens";
import { compareSortValues, type TableSort } from "@marshal/ui";
import { type Card, M } from "~/mock";

/** From this window width the Thinking, Permission mode, and Session columns show. */
export const DESKTOP_MIN_WIDTH_PX = breakpoints.lg;

/** Table minimum widths in px: the wide table has three more columns. */
export const WIDE_TABLE_MIN_WIDTH_PX = 1180;
export const NARROW_TABLE_MIN_WIDTH_PX = 820;

/** The Card column keeps this width so long titles do not squeeze the other columns. */
const CARD_COLUMN_CLASS = "w-[320px]";

const STATE_ORDER: readonly string[] = [
  "working",
  "needs",
  "planning",
  "merging",
  "review",
  "ready",
  "done",
  "backlog",
];

type AgentSortKey =
  | "card"
  | "role"
  | "agent"
  | "model"
  | "think"
  | "perm"
  | "state"
  | "sess"
  | "activity"
  | "cost";

export interface AgentColumn {
  /** The sort key. Empty for the Actions column, which cannot be sorted. */
  key: AgentSortKey | "";
  label: string;
  /** Hidden below the desktop width. */
  wideOnly?: boolean;
  class?: string;
}

const AGENT_COLUMNS: readonly AgentColumn[] = [
  { key: "card", label: "Card", class: CARD_COLUMN_CLASS },
  { key: "role", label: "Role" },
  { key: "agent", label: "Agent" },
  { key: "model", label: "Model" },
  { key: "think", label: "Thinking", wideOnly: true },
  { key: "perm", label: "Permission mode", wideOnly: true },
  { key: "state", label: "State" },
  { key: "sess", label: "Session", wideOnly: true },
  { key: "activity", label: "Current activity" },
  { key: "cost", label: "Cost" },
  { key: "", label: "Actions" },
];

/** The columns shown at this width. The column objects are shared, so the header rows keep their DOM. */
export function columnsFor(wide: boolean): readonly AgentColumn[] {
  return AGENT_COLUMNS.filter((column) => wide || !column.wideOnly);
}

/** What the Session column says about a card's agent process. */
export function sessionLabel(card: Card): string {
  if (card.waking) return "Waking";
  if (card.asleep) return "Asleep";
  return card.state === "done" ? "Stopped" : "Awake";
}

export function sessionIcon(card: Card): string {
  if (card.asleep) return "moon";
  return card.state === "done" ? "circle-stop" : "sun";
}

/** What the Current activity column says, whether or not it is shown as live. */
export function activityOf(card: Card): string {
  if (card.state === "needs") return card.reason;
  if (card.paused) return "Paused by you";
  return card.doing || (card.state === "done" ? "Merged" : "Idle");
}

type SortValue = string | number;

const SORT_VALUES: Record<AgentSortKey, (card: Card) => SortValue> = {
  card: (card) => card.id,
  role: (card) => card.role,
  agent: (card) => card.agent,
  model: (card) => card.model,
  think: (card) => M.THINK.indexOf(card.think ?? ""),
  perm: (card) => card.perm,
  state: (card) => STATE_ORDER.indexOf(card.state),
  sess: sessionLabel,
  activity: (card) => card.doing || card.reason || "",
  cost: (card) => card.cost,
};

const isSortKey = (key: string): key is AgentSortKey => Object.hasOwn(SORT_VALUES, key);

/**
 * Sorts a copy of the cards. Cards with equal values go newest first, and that tie order flips
 * with the direction, as in the prototype.
 */
export function sortAgentCards(cards: readonly Card[], sort: TableSort): Card[] {
  const sortValue = isSortKey(sort.k) ? SORT_VALUES[sort.k] : () => 0;
  return cards
    .slice()
    .sort((a, b) => (compareSortValues(sortValue(a), sortValue(b)) || b.id - a.id) * sort.dir);
}

/** The cards that have an agent session: everything except the backlog. */
export const withSessions = (cards: readonly Card[]): Card[] =>
  cards.filter((card) => card.state !== "backlog");
