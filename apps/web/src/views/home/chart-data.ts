import type { AxisTick, ChartSeries } from "@marshal/ui";
import { microsToDollars } from "~/data/mappers/limits";
import { addDays, dayOfWeek, formatShortDate } from "~/data/zone";
import type { DailyStats } from "~/mock/types";

/*
 * The numbers behind the two Home charts. The cost chart reads the daemon's own stored days (section
 * S19b); the cards chart reads them too once S19a is switched, and before that makes its past days up
 * from a seeded generator (the same day always gives the same value).
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
    const weekend = dayOfWeek(day * days.dayMs) % WEEKEND_DAY_STEP === 0;
    return Math.round(
      MIN_CARDS_PER_DAY + seeded(day) * CARDS_SPREAD + (weekend ? WEEKEND_ADJUST : WEEKDAY_ADJUST),
    );
  });
}

/** One cost line's project: the id its stored days are keyed by, and the name the line is labelled with. */
export interface ProjectCost {
  id: string;
  name: string;
}

/** The trailing `range` of a series, oldest first, padded with zeros when the daemon has fewer days. */
function trailing(values: readonly number[], range: number): number[] {
  const tail = values.slice(-range);
  return tail.length >= range ? tail : [...new Array<number>(range - tail.length).fill(0), ...tail];
}

/**
 * One cost line per project, oldest day first, and the total of all of them first in the list, read
 * from the daemon's own stored numbers (section S19b) rather than made up: the total is `stats.days`
 * and each project's line is its own stored series. Money is micro-dollars on the wire, so each day
 * is shown in dollars. A range the daemon has no days for draws as zeros.
 */
export function costSeries(
  stats: DailyStats,
  range: number,
  projects: readonly ProjectCost[],
): ChartSeries[] {
  const dollars = (micros: number): number => microsToDollars(micros);
  return [
    {
      name: "All projects",
      values: trailing(
        stats.days.map((day) => dollars(day.costMicros)),
        range,
      ),
    },
    ...projects.map((project) => {
      const days = stats.projects.find((row) => row.projectId === project.id)?.days ?? [];
      return {
        name: project.name,
        values: trailing(
          days.map((day) => dollars(day.costMicros)),
          range,
        ),
      };
    }),
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

/** Date labels under the charts, such as `Sep 18`. */
export function dateTicks(days: ChartDays): AxisTick[] {
  return tickIndexes(days.range).map((index) => ({
    index,
    label: formatShortDate(addDays(days.today, index - (days.range - 1))),
  }));
}

/** Hover text of each bar, such as `Sep 18: 3 cards`. */
export function barTips(days: ChartDays, values: readonly number[]): string[] {
  return values.map(
    (value, i) => `${formatShortDate(addDays(days.today, i - (days.range - 1)))}: ${value} cards`,
  );
}

const sum = (values: readonly number[]): number => values.reduce((a, b) => a + b, 0);

export function barSummary(values: readonly number[], range: number): string {
  return `${sum(values)} cards finished in the last ${range} days`;
}

/** Grid label of the cost axis, such as `$12`. */
export const dollarLabel = (value: number): string => `$${Math.round(value)}`;
