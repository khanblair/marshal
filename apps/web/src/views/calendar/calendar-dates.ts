/**
 * Date math of the calendar, from design/CalendarView.dc.html. Days are local calendar
 * days: cells step with `setDate`, so a daylight saving change never skips a day.
 */

export type CalMode = "month" | "week";

const DAYS_PER_WEEK = 7;
/** Six weeks, enough for any month starting on any weekday. */
const MONTH_GRID_DAYS = 42;
/** `getDay()` counts from Sunday; this makes Monday 0 and Sunday 6. */
const MONDAY_FIRST_SHIFT = 6;

/** Midnight of the Monday of the week that holds `t`. */
export function mondayOf(t: number): number {
  const d = new Date(t);
  const sinceMonday = (d.getDay() + MONDAY_FIRST_SHIFT) % DAYS_PER_WEEK;
  d.setDate(d.getDate() - sinceMonday);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

const firstOfMonth = (cursor: number): Date => {
  const c = new Date(cursor);
  return new Date(c.getFullYear(), c.getMonth(), 1);
};

export interface GridRange {
  /** Midnight of the first cell. */
  start: number;
  count: number;
}

/** Month: six weeks from the Monday before the 1st. Week: the cursor's week. */
export function gridRange(mode: CalMode, cursor: number): GridRange {
  if (mode === "month")
    return { start: mondayOf(firstOfMonth(cursor).getTime()), count: MONTH_GRID_DAYS };
  return { start: mondayOf(cursor), count: DAYS_PER_WEEK };
}

/** The day `i` days after `start`. */
export function dayAfter(start: number, i: number): Date {
  const d = new Date(start);
  d.setDate(d.getDate() + i);
  return d;
}

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

export const fullDate = (t: number): string =>
  new Date(t).toLocaleDateString(undefined, { dateStyle: "full" });

/** The grid's days. Days outside the cursor's month are marked in month mode only. */
export function gridCells(mode: CalMode, cursor: number, today: number): CalCell[] {
  const { start, count } = gridRange(mode, cursor);
  const month = new Date(cursor).getMonth();
  return Array.from({ length: count }, (_, i) => {
    const d = dayAfter(start, i);
    const t = d.getTime();
    return {
      t,
      key: String(t),
      date: d.getDate(),
      full: fullDate(t),
      today: t === today,
      inMonth: mode === "week" || d.getMonth() === month,
    };
  });
}

/** Column headings: "Mon", or "Mon 21" in week mode; one letter when narrow. */
export function weekdayLabels(mode: CalMode, cursor: number, narrow: boolean): string[] {
  const { start } = gridRange(mode, cursor);
  return Array.from({ length: DAYS_PER_WEEK }, (_, i) => {
    const d = dayAfter(start, i);
    const name = d.toLocaleDateString(undefined, { weekday: narrow ? "narrow" : "short" });
    return mode === "week" ? `${name} ${d.getDate()}` : name;
  });
}

/** The cursor one month or one week earlier (`dir` -1) or later (1). */
export function shiftCursor(mode: CalMode, cursor: number, dir: 1 | -1): number {
  const d = new Date(cursor);
  if (mode === "month") d.setMonth(d.getMonth() + dir);
  else d.setDate(d.getDate() + DAYS_PER_WEEK * dir);
  return d.getTime();
}

export const monthLabel = (cursor: number): string =>
  new Date(cursor).toLocaleDateString(undefined, { month: "long", year: "numeric" });

/** The heading: "September 2026", or "Week of September 21". Phones always show the month. */
export function titleLabel(mode: CalMode, cursor: number, phone: boolean): string {
  if (phone || mode === "month") return monthLabel(cursor);
  const start = new Date(gridRange(mode, cursor).start);
  return `Week of ${start.toLocaleDateString(undefined, { month: "long", day: "numeric" })}`;
}

/** Midnight of every day in the cursor's month. */
export function monthDays(cursor: number): number[] {
  const first = firstOfMonth(cursor);
  const count = new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate();
  return Array.from({ length: count }, (_, i) => dayAfter(first.getTime(), i).getTime());
}

/** Agenda heading: "Today, Thursday, September 24". */
export const agendaLabel = (t: number, today: number): string =>
  `${t === today ? "Today, " : ""}${new Date(t).toLocaleDateString(undefined, {
    weekday: "long",
    month: "long",
    day: "numeric",
  })}`;
