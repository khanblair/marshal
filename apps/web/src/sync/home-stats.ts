import {
  EventTypeActivityCreated,
  type HomeSnapshot,
  type Event as WireEvent,
  type HomeStatDay as WireHomeStatDay,
} from "@marshal/protocol";
import { batch } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import type { Ctx } from "~/mock/context";
import type { DailyStat, ProjectDailyStats } from "~/mock/types";
import { toStoredDailyStats, toStoredStatDay } from "./home-mapper";
import type { Syncer } from "./syncer";

/** How many days the syncer keeps: the widest range the range control offers. */
const WIDEST_RANGE_DAYS = 90;

/**
 * Section S19a: the stored numbers behind the Home charts ("cards finished per day"). They are the
 * daemon's `daily_stats`, read once from the dashboard answer and kept current by the
 * `activity.created` events of the home topic.
 *
 * The widest range is read, so the three ranges the range control offers are a slice of what is in
 * the store and changing the control needs no second call. A range is at most one stored row per
 * day per project, so this is a handful of numbers.
 *
 * The charts never count cards again: one read fills them, and a live change is one day of one
 * project.
 */
export const homeStatsSyncer: Syncer<HomeSnapshot> = {
  section: "S19a",
  topics: ["home"],
  async load(api: ApiClient) {
    return api.home({ range: WIDEST_RANGE_DAYS });
  },
  apply(ctx, snapshot) {
    ctx.S.stats = toStoredDailyStats(snapshot.stats);
  },
  onEvent: applyHomeStatsEvent,
};

/** One day with nothing on it, which is what a project that stored nothing that day looks like. */
const emptyDay = (day: number): DailyStat => ({
  day,
  cardsFinished: 0,
  merges: 0,
  ciFailures: 0,
  costMicros: 0,
});

/** Puts one day into a series: the day it is about, or the same series with it added. */
function putDay(days: DailyStat[], day: DailyStat): void {
  const index = days.findIndex((existing) => existing.day === day.day);
  if (index >= 0) days[index] = day;
  else days.push(day);
}

/** One project's series for the range, with only the day the event is about set. */
const blankSeries = (days: readonly DailyStat[], day: DailyStat): DailyStat[] =>
  days.map((template) => (template.day === day.day ? day : emptyDay(template.day)));

/**
 * The all-project totals of one day: the projects added together. The daemon's own totals are built
 * the same way, so a chart drawn from `days` and one drawn by adding `projects` up agree.
 */
function totalOf(projects: readonly ProjectDailyStats[], day: number): DailyStat {
  const total = emptyDay(day);
  for (const project of projects) {
    const own = project.days.find((stored) => stored.day === day);
    if (!own) continue;
    total.cardsFinished += own.cardsFinished;
    total.merges += own.merges;
    total.ciFailures += own.ciFailures;
    total.costMicros += own.costMicros;
  }
  return total;
}

/**
 * One day of one project changed, as `activity.created` carries it. The event carries the project's
 * whole day and not a difference, so applying it twice changes nothing.
 *
 * The event is applied only once the range has been loaded: a change that arrives before the first
 * snapshot is already in the stored numbers, so the load brings it and nothing is lost.
 */
export function applyHomeStatsEvent(ctx: Ctx, event: WireEvent): void {
  if (event.type !== EventTypeActivityCreated || !isRecord(event.data)) return;
  const projectId = projectOf(event.data.entry);
  const day = readDay(event.data.day);
  if (!projectId || !day) return;
  const stats = ctx.S.stats;
  if (stats.days.length === 0) return;
  batch(() => {
    const series = stats.projects.find((project) => project.projectId === projectId);
    if (series) putDay(series.days, day);
    else stats.projects.push({ projectId, days: blankSeries(stats.days, day) });
    // The totals follow the project that changed, so both charts stay in step.
    const totals = stats.days.map((stored) => totalOf(stats.projects, stored.day));
    totals.forEach((total, index) => {
      stats.days[index] = total;
    });
  });
}

/** The project an entry is about, which is the project the day beside it belongs to, or empty. */
function projectOf(entry: unknown): string {
  if (!isRecord(entry) || typeof entry.projectId !== "string") return "";
  return entry.projectId;
}

/** The stored day an `activity.created` event carries, or null when it carries none. */
function readDay(day: unknown): DailyStat | null {
  if (!isRecord(day) || typeof day.day !== "string") return null;
  return toStoredStatDay(day as unknown as WireHomeStatDay);
}
