/**
 * Geometry of the timeline grid, value for value from design/TimelineView.dc.html.
 * Everything here is a pure function of day offsets (days from today) and row indexes.
 */

/** First day on the grid, as an offset from today. */
const START_DAY = -14;
/** Number of days on the grid. */
export const DAY_COUNT = 32;
const DAY_WIDTH_PX = 40;
/** Width of the sticky card column left of the track. */
export const CARD_COLUMN_PX = 280;
/**
 * Vertical step between dependency lines. Rows are 44 px tall including their bottom
 * border (the app uses border-box), but the design spaces the lines 45 px apart, so a
 * line drifts down one pixel per row. Kept exactly as drawn.
 */
const LINE_ROW_PX = 45;
/** A line's height inside its row. */
const LINE_MID_PX = 22;
/** Gap between a bar and the day boundaries it spans. */
const BAR_INSET_PX = 2;
const BAR_MIN_WIDTH_PX = 24;
/** The elbow of a dependency line sits this far right of where the line starts. */
const ELBOW_PX = 10;
/** The today line runs down the middle of today's column. */
const TODAY_LINE_OFFSET_PX = 20;
/** Moving the pointer farther than this makes a press a drag instead of a click. */
export const DRAG_MOVE_PX = 4;
const SUNDAY = 0;
const SATURDAY = 6;

export const TRACK_WIDTH_PX = DAY_COUNT * DAY_WIDTH_PX;
export const TOTAL_WIDTH_PX = CARD_COLUMN_PX + TRACK_WIDTH_PX;
export const TODAY_LINE_X_PX =
  CARD_COLUMN_PX + (0 - START_DAY) * DAY_WIDTH_PX + TODAY_LINE_OFFSET_PX;

/** Planned start and end day of a card, both inclusive. */
export interface Span {
  s: number;
  e: number;
}

/** What the timeline needs from a card. */
export interface Planned {
  id: number;
  s: number | null;
  e: number | null;
  deps: readonly number[];
}

/** A bar being dragged: its card and how many days it moved. */
export interface BarDrag {
  id: number;
  delta: number;
}

export const px = (value: number): string => `${value}px`;

/**
 * The card's span, shifted while its bar is dragged. A missing day counts as 0, the way
 * the design's arithmetic treats `null`.
 */
export function spanOf(c: Planned, drag: BarDrag | null): Span {
  const shift = drag && drag.id === c.id ? drag.delta : 0;
  return { s: (c.s ?? 0) + shift, e: (c.e ?? 0) + shift };
}

export const barX = (span: Span): number => (span.s - START_DAY) * DAY_WIDTH_PX + BAR_INSET_PX;

export const barWidth = (span: Span): number =>
  Math.max(BAR_MIN_WIDTH_PX, (span.e - span.s + 1) * DAY_WIDTH_PX - 2 * BAR_INSET_PX);

/** Whole days a horizontal pointer move stands for. */
export const snapDays = (dx: number): number => Math.round(dx / DAY_WIDTH_PX);

export const svgHeight = (rows: number): number => rows * LINE_ROW_PX;

export interface DepLine {
  /** Path from the end of the blocking bar to the start of the waiting one. */
  d: string;
  /** The waiting card starts before the blocking card ends. */
  bad: boolean;
}

/** The line from card `from` (row `fromRow`) to card `to` (row `toRow`) that waits for it. */
export function depLine(from: Span, fromRow: number, to: Span, toRow: number): DepLine {
  const x1 = (from.e - START_DAY + 1) * DAY_WIDTH_PX - BAR_INSET_PX;
  const y1 = fromRow * LINE_ROW_PX + LINE_MID_PX;
  const x2 = (to.s - START_DAY) * DAY_WIDTH_PX + BAR_INSET_PX;
  const y2 = toRow * LINE_ROW_PX + LINE_MID_PX;
  const mx = Math.max(x1 + ELBOW_PX, Math.min(x2 - ELBOW_PX, x1 + ELBOW_PX));
  return { d: `M${x1} ${y1} H${mx} V${y2} H${x2}`, bad: to.s <= from.e };
}

/**
 * Every dependency line between cards on the grid, in the design's order: by waiting
 * card, then by its dependency list. Dependencies not on the grid draw nothing.
 */
export function dependencyLines<T extends Planned>(
  rows: readonly T[],
  span: (c: T) => Span,
): DepLine[] {
  const index = new Map(rows.map((c, i) => [c.id, i]));
  return rows.flatMap((b, bi) =>
    b.deps.flatMap((aid) => {
      const ai = index.get(aid);
      const a = ai === undefined ? undefined : rows[ai];
      return a === undefined || ai === undefined ? [] : [depLine(span(a), ai, span(b), bi)];
    }),
  );
}

export interface DayCell {
  x: number;
  weekday: string;
  date: number;
  full: string;
  today: boolean;
  weekend: boolean;
}

/** Midnight of a day on the grid. Adds whole days in ms, as the design does. */
const dayStart = (today: number, dayMs: number, offset: number): Date =>
  new Date(today + offset * dayMs);

/** The day header cells. */
export function dayCells(today: number, dayMs: number): DayCell[] {
  return Array.from({ length: DAY_COUNT }, (_, i) => {
    const t = dayStart(today, dayMs, START_DAY + i);
    const dow = t.getDay();
    return {
      x: i * DAY_WIDTH_PX,
      weekday: t.toLocaleDateString(undefined, { weekday: "narrow" }),
      date: t.getDate(),
      full: t.toLocaleDateString(undefined, { dateStyle: "full" }),
      today: START_DAY + i === 0,
      weekend: dow === SUNDAY || dow === SATURDAY,
    };
  });
}

/** "Sep 10 to Oct 11": the first and last day on the grid. */
export function rangeLabel(today: number, dayMs: number): string {
  const f = (offset: number) =>
    dayStart(today, dayMs, offset).toLocaleDateString(undefined, {
      month: "short",
      day: "numeric",
    });
  return `${f(START_DAY)} to ${f(START_DAY + DAY_COUNT - 1)}`;
}
