import { type Card, type Column, M, type SwimKey } from "~/mock";
import type { CardKey } from "~/mock/card-key";

/** Done shows only the newest cards until "Show all". */
export const DONE_LIMIT = 20;
const NO_LANE_PREFIX = "No ";
export const NO_PACKAGE = "No package";
const NO_LABEL = "No label";
/** The single lane of a board without swimlanes (or with no cards). */
export const ALL_LANE = "all";
const QUICK_ADD_COLUMNS: readonly Column[] = ["backlog", "planning", "working"];

export interface ColumnModel {
  col: Column;
  /** The cards to draw: Done is cut to the newest 20 unless `showAllDone`. */
  cards: Card[];
  /** All cards of the column in this lane, before the Done limit. */
  total: number;
  showAll: boolean;
}

export interface LaneModel {
  key: string;
  count: number;
  collapsed: boolean;
  /** Empty for a collapsed lane, which draws no columns. */
  columns: ColumnModel[];
}

export interface BoardModel {
  lanes: LaneModel[];
  /** Card keys per column index, the keyboard navigation model the shell reads from `M.nav`. */
  grid: CardKey[][];
}

/** Where an add card control lives: a column inside a lane. */
export interface ColumnRef {
  laneKey: string;
  col: Column;
}

export interface BoardInput {
  list: readonly Card[];
  /** The store may hold no value for a project, which behaves like a lane per "all". */
  swim: SwimKey | undefined;
  packages: readonly string[];
  columns: readonly Column[];
  isCollapsed: (laneKey: string) => boolean;
  showAllDone: boolean;
}

export function laneKeyOf(card: Card, swim: SwimKey | undefined): string {
  switch (swim) {
    case "role":
      return card.role;
    case "agent":
      return card.agent;
    case "package":
      return card.pkg || NO_PACKAGE;
    case "label":
      return card.labels[0] || NO_LABEL;
    default:
      return ALL_LANE;
  }
}

/** "No package" and "No label" sort last. */
const compareLaneKeys = (a: string, b: string): number =>
  Number(a.startsWith(NO_LANE_PREFIX)) - Number(b.startsWith(NO_LANE_PREFIX)) || a.localeCompare(b);

/**
 * The lanes in order. Package lanes follow the project's package order, then any package a card
 * names that the project does not list (the daemon lists what it found on disk, and a card can
 * name another), each in its own lane, then "No package". No card is left without a lane.
 */
export function laneKeys(
  list: readonly Card[],
  swim: SwimKey | undefined,
  packages: readonly string[],
): string[] {
  const seen = Array.from(new Set(list.map((c) => laneKeyOf(c, swim)))).sort(compareLaneKeys);
  const keys =
    swim === "package"
      ? [
          ...packages.filter((k) => seen.includes(k)),
          ...seen.filter((k) => k !== NO_PACKAGE && !packages.includes(k)),
          ...seen.filter((k) => k === NO_PACKAGE),
        ]
      : seen;
  return keys.length ? keys : [ALL_LANE];
}

function buildColumn(cards: readonly Card[], col: Column, showAllDone: boolean): ColumnModel {
  const all = M.colCards(cards, col);
  const limited = col === "done" && !showAllDone;
  return {
    col,
    cards: limited ? all.slice(0, DONE_LIMIT) : all,
    total: all.length,
    showAll: limited && all.length > DONE_LIMIT,
  };
}

/** Lanes, their columns, and the keyboard grid for the filtered cards of one project. */
export function buildBoard(input: BoardInput): BoardModel {
  const grid: CardKey[][] = input.columns.map(() => []);
  const lanes = laneKeys(input.list, input.swim, input.packages).map((key): LaneModel => {
    const inLane = input.list.filter((c) => laneKeyOf(c, input.swim) === key);
    const collapsed = input.isCollapsed(key);
    const columns = collapsed
      ? []
      : input.columns.map((col) => buildColumn(inLane, col, input.showAllDone));
    columns.forEach((column, index) => {
      grid[index]?.push(...column.cards.map((c) => c.id));
    });
    return { key, count: inLane.length, collapsed, columns };
  });
  return { lanes, grid };
}

/** Cards of the list per column, for the column headers and the phone tabs. */
export function countByColumn(list: readonly Card[]): Record<Column, number> {
  const counts = Object.fromEntries(M.COLUMNS.map((col) => [col, 0])) as Record<Column, number>;
  for (const card of list) counts[M.colOf(card.state)] += 1;
  return counts;
}

export const canQuickAdd = (col: Column): boolean => QUICK_ADD_COLUMNS.includes(col);

const ADD_HINTS: Partial<Record<Column, string>> = {
  backlog: "The card waits in Backlog until you start it.",
  planning: "The agent starts in plan first mode and posts a plan for you.",
};

export const addHint = (col: Column): string =>
  ADD_HINTS[col] ?? "The agent starts working on this card right away.";

export const addLabel = (col: Column): string => (col === "backlog" ? "Add card" : "Add and start");

export const cardCountLabel = (count: number): string =>
  `${count} ${count === 1 ? "card" : "cards"}`;

/** What an empty column says: a drop hint while dragging, else "No cards" on a plain board. */
export function emptyText(dragging: boolean, swim: SwimKey | undefined): string {
  if (dragging) return "Drop here";
  return swim === "none" ? "No cards" : "";
}

/** Tailwind class for a column's minimum height: tall on a plain board, short inside lanes. */
export function columnMinHeightClass(swim: SwimKey | undefined, mobile: boolean): string {
  if (swim !== "none") return "min-h-18";
  return mobile ? "min-h-60" : "min-h-110";
}
