import type {
  FeedEntry as WireFeedEntry,
  HomeProjectStats as WireHomeProjectStats,
  HomeStatDay as WireHomeStatDay,
  HomeStats as WireHomeStats,
} from "@marshal/protocol";
import { toMillis } from "~/data/mappers/time";
import type { DailyStat, DailyStats, FeedItem, ProjectDailyStats } from "~/mock/types";

/*
 * The Home numbers and the activity stream, from the daemon to the screens.
 *
 * The two sides were made to look alike, so this is one word for another in most places. What is
 * here is the few places they are not:
 *
 *   - The store names an entry's project `pid` and the list itself `feed`, because that is what the
 *     mock called them and the feed's rows have read them since before the daemon existed.
 *   - A feed entry points at what it opens the way the store does: a card by its key (`api#41`),
 *     which is what the views look up, and a scheduled job by its id. An entry about neither
 *     carries neither.
 *   - A stored day is a wire timestamp on the daemon and a number of milliseconds here, which is
 *     what the chart's axis arithmetic wants.
 */

/** One entry of the activity stream, in the shape the feed draws. */
export function toStoredFeedItem(entry: WireFeedEntry): FeedItem {
  return {
    id: entry.id,
    kind: entry.kind,
    text: entry.text,
    // The wire leaves the project out when there is none, and the store says so with null.
    pid: entry.projectId ?? null,
    ts: toMillis(entry.at),
    // Absent rather than empty, so `openFeedItem` and the row's own lookups see "no card" and "no
    // job" exactly as they did for a mock entry that had none.
    ...(entry.cardKey ? { cardId: entry.cardKey } : {}),
    ...(entry.jobId ? { job: entry.jobId } : {}),
  };
}

/**
 * One page of the activity stream, in the order the screens draw it. The daemon answers newest
 * first and so does the feed, so the page is not reversed here.
 */
export const toStoredFeedItems = (page: readonly WireFeedEntry[]): FeedItem[] =>
  page.map(toStoredFeedItem);

/** One stored day, with its time as milliseconds. */
export function toStoredStatDay(day: WireHomeStatDay): DailyStat {
  return {
    day: toMillis(day.day),
    cardsFinished: day.cardsFinished,
    merges: day.merges,
    ciFailures: day.ciFailures,
    costMicros: day.costMicros,
  };
}

/** One project's own days. */
const toStoredProjectStats = (project: WireHomeProjectStats): ProjectDailyStats => ({
  projectId: project.projectId,
  days: project.days.map(toStoredStatDay),
});

/**
 * The stored numbers the Home charts draw. A missing or null answer (a hand-built snapshot in a
 * test, or one the daemon did not fill) becomes an empty range rather than `undefined`, so a chart
 * reads a list and never checks.
 */
export function toStoredDailyStats(stats: WireHomeStats | null | undefined): DailyStats {
  if (!stats) return { range: 0, days: [], projects: [] };
  return {
    range: stats.range,
    days: stats.days.map(toStoredStatDay),
    projects: stats.projects.map(toStoredProjectStats),
  };
}
