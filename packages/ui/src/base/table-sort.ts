import type { SortDirection } from "./SortHeader";

/** Which column a table is sorted by, and which way: 1 ascending, -1 descending. */
export interface TableSort {
  k: string;
  dir: number;
}

/** The `aria-sort` value of a column header. */
export function sortDirection(current: TableSort, key: string): SortDirection {
  if (current.k !== key) return "none";
  return current.dir > 0 ? "ascending" : "descending";
}

/** A click on a header: the same column flips direction, another column starts ascending. */
export function toggleSort(current: TableSort, key: string): TableSort {
  return { k: key, dir: current.k === key ? -current.dir : 1 };
}

/** Orders two values of one column: numbers by value, text by code unit (not by locale). */
export function compareSortValues(x: string | number, y: string | number): number {
  if (x > y) return 1;
  return x < y ? -1 : 0;
}
