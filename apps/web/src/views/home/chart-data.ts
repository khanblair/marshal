import type { AxisTick, ChartSeries } from "@marshal/ui";
import type { DailyStats } from "~/mock/types";

/*
 * The numbers behind the two Home charts. The design has no history, so past
 * days are made up from a seeded generator (the same day always gives the same
 * value) and only today's value comes from the store.
 */

/** The days a chart covers: `today` is the store's midnight of today, `dayMs` the length of a day. */
export interface ChartDays {
  today: number;
  dayMs: number;
  /** How many days the chart shows, ending today. */
  range: number;
}

const SEED_MULTIPLIER = 12.9898;
const SEED_OFFSET = 78.233;
const SEED_SCALE = 43758.5453;

/** A repeatable number in [0, 1) for a seed, the classic sine hash. */
export function seeded(seed: number): number {
  const x = Math.sin(seed * SEED_MULTIPLIER + SEED_OFFSET) * SEED_SCALE;
  return x - Math.floor(x);
}

/** The whole-day number (days since 1970) of the chart position `i`, where the last position is today. */
function dayNumber(days: ChartDays, i: number): number {
  return Math.floor(days.today / days.dayMs) - (days.range - 1 - i);
}

const MIN_CARDS_PER_DAY = 1;
const CARDS_SPREAD = 5;
/** Saturdays and Sundays finish one card fewer, the others one more. */
const WEEKEND_DAY_STEP = 6;
const WEEKEND_ADJUST = -1;
const WEEKDAY_ADJUST = 1;

/**
 * Cards finished on each day, oldest first, read from the daemon's own stored numbers (section
 * S19a) rather than made up: `stats.days` always holds the widest range the syncer asked for, so
 * this takes the trailing `range` days of it.
 */
export function finishedFromStats(stats: DailyStats, range: number): number[] {
  return stats.days.slice(-range).map((day) => day.cardsFinished);
}

/** Cards finished on each day, oldest first. Today's value is the real count of merges. */
export function finishedPerDay(days: ChartDays, mergedToday: number): number[] {
  return Array.from({ length: days.range }, (_, i) => {
    if (i === days.range - 1) return mergedToday;
    const day = dayNumber(days, i);
    const weekend = new Date(day * days.dayMs).getDay() % WEEKEND_DAY_STEP === 0;
    return Math.round(
      MIN_CARDS_PER_DAY + seeded(day) * CARDS_SPREAD + (weekend ? WEEKEND_ADJUST : WEEKDAY_ADJUST),
    );
  });
}

/** Typical daily spend in dollars of the seeded projects; others count as `DEFAULT_DAILY_COST_USD`. */
const DAILY_COST_USD: Record<string, number> = { api: 3.6, web: 2.8, mobile: 5.4 };
const DEFAULT_DAILY_COST_USD = 2;
const MIN_DAILY_COST_USD = 0.2;
const COST_FLOOR_RATIO = 0.55;
const COST_SPREAD_RATIO = 0.9;
/** Spreads the seeds of neighbouring days and different projects apart. */
const COST_SEED_STRIDE = 3;

export interface ProjectCost {
  id: string;
  name: string;
  /** What the project has spent today, in dollars. */
  todayCost: number;
}

/**
 * One cost line per project, oldest day first, and the total of all of them
 * first in the list. Today's point is the project's real spend.
 */
export function costSeries(days: ChartDays, projects: readonly ProjectCost[]): ChartSeries[] {
  const lines = projects.map((project) =>
    Array.from({ length: days.range }, (_, i) => {
      if (i === days.range - 1) return project.todayCost;
      const typical = DAILY_COST_USD[project.id] || DEFAULT_DAILY_COST_USD;
      const seed = dayNumber(days, i) * COST_SEED_STRIDE + project.id.length;
      return Math.max(
        MIN_DAILY_COST_USD,
        typical * (COST_FLOOR_RATIO + seeded(seed) * COST_SPREAD_RATIO),
      );
    }),
  );
  const total = Array.from({ length: days.range }, (_, i) =>
    lines.reduce((sum, values) => sum + (values[i] ?? 0), 0),
  );
  return [
    { name: "All projects", values: total },
    ...projects.map((project, i) => ({ name: project.name, values: lines[i] ?? [] })),
  ];
}

const WEEK_DAYS = 7;
const MONTH_DAYS = 30;
// biome-ignore-start lint/style/noMagicNumbers: the design's tick positions, as a table
const WEEK_TICKS: readonly number[] = [0, 3, 6];
const MONTH_TICKS: readonly number[] = [0, 10, 20, 29];
const QUARTER_TICKS: readonly number[] = [0, 30, 60, 89];
// biome-ignore-end lint/style/noMagicNumbers: the design's tick positions, as a table

/** The chart positions that get a date label: three or four spread over the range. */
export function tickIndexes(range: number): number[] {
  if (range === WEEK_DAYS) return [...WEEK_TICKS];
  return [...(range === MONTH_DAYS ? MONTH_TICKS : QUARTER_TICKS)];
}

const shortDay = (ts: number): string =>
  new Date(ts).toLocaleDateString(undefined, { month: "short", day: "numeric" });

/** Date labels under the charts, such as `Sep 18`. */
export function dateTicks(days: ChartDays): AxisTick[] {
  return tickIndexes(days.range).map((index) => ({
    index,
    label: shortDay(days.today - (days.range - 1 - index) * days.dayMs),
  }));
}

/** Hover text of each bar, such as `Sep 18: 3 cards`. */
export function barTips(days: ChartDays, values: readonly number[]): string[] {
  return values.map((value, i) => `${shortDay(dayNumber(days, i) * days.dayMs)}: ${value} cards`);
}

const sum = (values: readonly number[]): number => values.reduce((a, b) => a + b, 0);

export function barSummary(values: readonly number[], range: number): string {
  return `${sum(values)} cards finished in the last ${range} days`;
}

/** Grid label of the cost axis, such as `$12`. */
export const dollarLabel = (value: number): string => `$${Math.round(value)}`;
