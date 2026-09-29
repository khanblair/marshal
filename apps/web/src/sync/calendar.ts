import type { CalendarList } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { toCalEvents } from "~/data/mappers/calendar";
import type { Ctx } from "~/mock/context";
import type { Syncer } from "./syncer";

const DAY_MS = 86_400_000;
/** How far back and ahead of today the calendar reads, wide enough for the month view and the
 * coming-up list alike, without paging as a person scrolls. */
const DAYS_BACK = 14;
const DAYS_AHEAD = 45;

/**
 * Google Calendar's events (section S25, B8.4, N21). The schedules and the due cards this same
 * daemon call answers are read through their own syncers instead (schedulesSyncer, cardsSyncer),
 * so this one only ever touches `calEvents`.
 */
export const calendarSyncer: Syncer<CalendarList> = {
  section: "S25",
  topics: [],
  async load(api: ApiClient, ctx: Ctx) {
    const start = ctx.today - DAYS_BACK * DAY_MS;
    const end = ctx.today + DAYS_AHEAD * DAY_MS;
    return api.getCalendar(start, end);
  },
  apply(ctx, list) {
    ctx.S.calEvents = toCalEvents(list, ctx.today);
  },
};
