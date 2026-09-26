/**
 * The arithmetic behind `VirtualList`: which rows of a tall list to draw for the part of it that is
 * on screen. Rows have their own heights, and the list leaves `gap` px between neighbours. All
 * distances are px from the top of the list.
 */

/** The rows to draw, first to last, both included. `last` is below `first` for a list with no rows. */
export interface RowRange {
  first: number;
  last: number;
}

export interface WindowView {
  /** Where each row starts, and one more entry (`rowOffsets`). */
  offsets: readonly number[];
  /** How far the list is scrolled: the distance from its top to the top edge of what is on screen. */
  scrollTop: number;
  /** The height of what is on screen. */
  viewportHeight: number;
  /** Extra px drawn above and below the screen, so a scroll does not show blank space first. */
  overscan: number;
}

/**
 * Where each row starts, plus one more entry after the last: where a row after it would start,
 * with its gap. `heights[i]` is the height of row `i`.
 */
export function rowOffsets(heights: readonly number[], gap: number): number[] {
  const offsets: number[] = [];
  let top = 0;
  for (const height of heights) {
    offsets.push(top);
    top += height + gap;
  }
  offsets.push(top);
  return offsets;
}

const rowCount = (offsets: readonly number[]): number => Math.max(0, offsets.length - 1);

const startOf = (offsets: readonly number[], row: number): number => offsets[row] ?? 0;

/** The whole list's height: every row and the gaps between them, and no gap after the last. */
export function listHeight(offsets: readonly number[], gap: number): number {
  return rowCount(offsets) === 0 ? 0 : startOf(offsets, rowCount(offsets)) - gap;
}

/** How many rows start before `limit` (or at it, when `inclusive`), by binary search. */
function rowsStartingBy(offsets: readonly number[], limit: number, inclusive: boolean): number {
  let low = 0;
  let high = rowCount(offsets);
  while (low < high) {
    const middle = Math.floor((low + high) / 2);
    const start = startOf(offsets, middle);
    if (inclusive ? start <= limit : start < limit) low = middle + 1;
    else high = middle;
  }
  return low;
}

/**
 * The rows that touch the band from `top` to `bottom`. A row the band starts inside is in, and a
 * band below the end of the list still has the last row, so a list scrolled too far is not blank.
 */
export function rowsBetween(offsets: readonly number[], top: number, bottom: number): RowRange {
  if (rowCount(offsets) === 0) return { first: 0, last: -1 };
  const first = Math.max(0, rowsStartingBy(offsets, top, true) - 1);
  const last = Math.max(first, rowsStartingBy(offsets, bottom, false) - 1);
  return { first, last };
}

/** The rows to draw for a view of the list: the ones on screen and the overscan around them. */
export function rowWindow(view: WindowView): RowRange {
  const top = view.scrollTop - view.overscan;
  const bottom = view.scrollTop + view.viewportHeight + view.overscan;
  return rowsBetween(view.offsets, top, bottom);
}

/** True when two ranges name the same rows, so a scroll inside one range changes nothing. */
export const sameRange = (a: RowRange, b: RowRange): boolean =>
  a.first === b.first && a.last === b.last;

/**
 * The rows to draw, in order: the range, and also `keep` (the row that has focus) when it is
 * outside it, so scrolling away never removes the element a person is using.
 */
export function rowsToDraw(range: RowRange, keep: number | null, rows: number): number[] {
  const drawn: number[] = [];
  for (let row = range.first; row <= range.last; row++) drawn.push(row);
  if (keep === null || keep < 0 || keep >= rows) return drawn;
  if (keep < range.first) drawn.unshift(keep);
  else if (keep > range.last) drawn.push(keep);
  return drawn;
}

/** The scroll position that puts a row at the top of the screen, for "scroll to this file". */
export function scrollTopOfRow(offsets: readonly number[], row: number): number {
  const last = Math.max(0, rowCount(offsets) - 1);
  return startOf(offsets, Math.min(Math.max(0, row), last));
}
