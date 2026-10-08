import {
  addDays,
  addMonths,
  dayNumber,
  dayOfWeek,
  daysInMonth,
  formatDate,
  monthNumber,
  startOfDay,
  startOfMonth,
} from "~/data/zone";

/**
 * Date math of the calendar, from design/CalendarView.dc.html. Days are calendar days of the chosen
 * time zone: cells step by whole days, so a daylight saving change never skips a day.
 */

export type CalMode = "month" | "week";

const DAYS_PER_WEEK = 7;
/** Six weeks, enough for any month starting on any weekday. */
const MONTH_GRID_DAYS = 42;
/** The weekday counts from Sunday; this makes Monday 0 and Sunday 6. */
const MONDAY_FIRST_SHIFT = 6;

/** Midnight of the Monday of the week that holds `t`. */
export function mondayOf(t: number): number {
  const sinceMonday = (dayOfWeek(t) + MONDAY_FIRST_SHIFT) % DAYS_PER_WEEK;
  return startOfDay(addDays(t, -sinceMonday));
}

export interface GridRange {
  /** Midnight of the first cell. */
  start: number;
  count: number;
}

/** Month: six weeks from the Monday before the 1st. Week: the cursor's week. */
export function gridRange(mode: CalMode, cursor: number): GridRange {
  if (mode === "month") return { start: mondayOf(startOfMonth(cursor)), count: MONTH_GRID_DAYS };
  return { start: mondayOf(cursor), count: DAYS_PER_WEEK };
}

/** Midnight of the day `i` days after the day starting at `start`. */
export const dayAfter = (start: number, i: number): number => startOfDay(addDays(start, i));

export interface CalCell {
  /** Midnight of the day. */
  t: number;
  /** `S.calExpand` holds this while the day shows all its items. */
  key: string;
  date: number;
  full: string;
  today: boolean;
  inMonth: boolean;
}

export const fullDate = (t: number): string => formatDate(t, { dateStyle: "full" });

/** The grid's days. Days outside the cursor's month are marked in month mode only. */
export function gridCells(mode: CalMode, cursor: number, today: number): CalCell[] {
  const { start, count } = gridRange(mode, cursor);
  const month = monthNumber(cursor);
  return Array.from({ length: count }, (_, i) => {
    const t = dayAfter(start, i);
    return {
      t,
      key: String(t),
      date: dayNumber(t),
      full: fullDate(t),
      today: t === today,
      inMonth: mode === "week" || monthNumber(t) === month,
    };
  });
}

/** Column headings: "Mon", or "Mon 21" in week mode; one letter when narrow. */
export function weekdayLabels(mode: CalMode, cursor: number, narrow: boolean): string[] {
  const { start } = gridRange(mode, cursor);
  return Array.from({ length: DAYS_PER_WEEK }, (_, i) => {
    const t = dayAfter(start, i);
    const name = formatDate(t, { weekday: narrow ? "narrow" : "short" });
    return mode === "week" ? `${name} ${dayNumber(t)}` : name;
  });
}

/** The cursor one month or one week earlier (`dir` -1) or later (1). */
export function shiftCursor(mode: CalMode, cursor: number, dir: 1 | -1): number {
  return mode === "month" ? addMonths(cursor, dir) : addDays(cursor, DAYS_PER_WEEK * dir);
}

export const monthLabel = (cursor: number): string =>
  formatDate(cursor, { month: "long", year: "numeric" });

/** The heading: "September 2026", or "Week of September 21". Phones always show the month. */
export function titleLabel(mode: CalMode, cursor: number, phone: boolean): string {
  if (phone || mode === "month") return monthLabel(cursor);
  return `Week of ${formatDate(gridRange(mode, cursor).start, { month: "long", day: "numeric" })}`;
}

/** Midnight of every day in the cursor's month. */
export function monthDays(cursor: number): number[] {
  const first = startOfMonth(cursor);
  return Array.from({ length: daysInMonth(cursor) }, (_, i) => dayAfter(first, i));
}

/** Agenda heading: "Today, Thursday, September 24". */
export const agendaLabel = (t: number, today: number): string =>
  `${t === today ? "Today, " : ""}${formatDate(t, {
    weekday: "long",
    month: "long",
    day: "numeric",
  })}`;
