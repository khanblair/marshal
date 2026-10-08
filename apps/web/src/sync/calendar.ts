import type { CalendarList } from "@marshal/protocol";
import { createEffect, on } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { toCalEvents, toCalGoogle } from "~/data/mappers/calendar";
import type { Ctx } from "~/mock/context";
import type { Syncer } from "./syncer";

const DAY_MS = 86_400_000;
/** How far back and ahead of today the calendar reads, wide enough for the month view and the
 * coming-up list alike, without paging as a person scrolls. */
const DAYS_BACK = 14;
const DAYS_AHEAD = 45;
/** How often the events are read again while the app is open. The daemon keeps a read for a minute,
 * so this never costs Google a call each time. */
const REFRESH_MS = 300_000;
/** The fewest milliseconds between two reads set off by the window coming back into view. */
const FOCUS_GAP_MS = 20_000;
/** The id of the Google Calendar connection's row. */
const GCAL_ID = "gcal";

async function load(api: ApiClient, ctx: Ctx): Promise<CalendarList> {
  const start = ctx.today - DAYS_BACK * DAY_MS;
  const end = ctx.today + DAYS_AHEAD * DAY_MS;
  return api.getCalendar(start, end);
}

function apply(ctx: Ctx, list: CalendarList): void {
  ctx.S.calEvents = toCalEvents(list, ctx.today);
  ctx.S.calGoogle = toCalGoogle(list);
}

/**
 * Google Calendar's events (section S25, B8.4, N21). The schedules and the due cards this same
 * daemon call answers are read through their own syncers instead (schedulesSyncer, cardsSyncer),
 * so this one only ever touches `calEvents` and how the read went (`calGoogle`).
 *
 * Between snapshots it reads again every few minutes, when the window comes back into view (a
 * person who just granted access in another tab sees the events at once), and when the Google
 * Calendar connection's status changes. A read that fails leaves what was there.
 */
export const calendarSyncer: Syncer<CalendarList> = {
  section: "S25",
  topics: [],
  load,
  apply,
  start(ctx, api) {
    let last = 0;
    const refresh = (): void => {
      last = Date.now();
      void load(api, ctx).then(
        (list) => apply(ctx, list),
        () => undefined,
      );
    };
    const timer = setInterval(refresh, REFRESH_MS);
    // The window is cut from today, so a new day or a new time zone reads it again.
    createEffect(on(() => ctx.today, refresh, { defer: true }));
    const onVisible = (): void => {
      if (document.visibilityState === "visible" && Date.now() - last > FOCUS_GAP_MS) refresh();
    };
    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener("focus", onVisible);
    createEffect(
      on(
        () => ctx.S.integrations.find((row) => row.id === GCAL_ID)?.st,
        (status, before) => {
          if (before !== undefined && status !== before) refresh();
        },
      ),
    );
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener("focus", onVisible);
    };
  },
};
