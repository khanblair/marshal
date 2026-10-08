import type { CalendarList, CalendarEvent as WireCalendarEvent } from "@marshal/protocol";
import { clock, dateKey, daysBetween, daysBetweenDates, startOfDay } from "../zone";

/** One Google Calendar event, as the Calendar view and Home's coming-up list draw it. */
export interface CalEvent {
  id: string;
  title: string;
  /** `09:30`, on the clock of the chosen time zone. Empty for an all-day event, which has no time of day. */
  time: string;
  /** `10:00`, when the event ends the same day. Empty when it has no end, or runs past midnight. */
  endTime?: string;
  allDay?: boolean;
  location?: string;
  /** Opens the event in Google Calendar. */
  url?: string;
  /** The video call link, when the event has one. */
  joinUrl?: string;
  /** The name of the calendar the event is on. */
  calendar?: string;
  days?: number[];
  dayOffset?: number;
  /** The last day the event covers, as an offset from today. Absent when it is on one day only. */
  lastDayOffset?: number;
}

/**
 * How reading Google Calendar went. `known` is false until the daemon has answered, so a screen
 * never says "not connected" about a Google it has not asked yet.
 */
export interface CalGoogle {
  known: boolean;
  connected: boolean;
  /** Why Google's events are missing or old, in words a person can act on. Empty when all is well. */
  error: string;
  /** The events shown are the last ones read, because Google could not be reached just now. */
  stale: boolean;
}

/**
 * The last day an event covers, as an offset from today, or undefined when it stays on its first day.
 * A timed event ends at its end time, so one that ends at midnight is not on the next day: this looks
 * at the moment before the end.
 */
function lastDayOffsetOf(start: number, end: number | null, firstOffset: number, today: number) {
  if (end === null || end <= start) return undefined;
  const last = daysBetween(today, end - 1);
  return last > firstOffset ? last : undefined;
}

/**
 * The days an all-day event covers from its own dates, "2026-10-09" to the first day it no longer
 * covers. They belong to no zone, so the event is on that date wherever the viewer is. Null when the
 * daemon sent none.
 */
function allDayOffsets(wire: WireCalendarEvent, today: number) {
  const todayKey = dateKey(today);
  const first = daysBetweenDates(todayKey, wire.startDate ?? "");
  if (first === null) return null;
  const after = daysBetweenDates(todayKey, wire.endDate ?? "");
  const last = after === null ? undefined : after - 1;
  return { first, last: last !== undefined && last > first ? last : undefined };
}

/**
 * One event, with its wire `start` (an absolute moment) turned into `time` and `dayOffset` - the
 * relative shape every screen that draws a calendar already reads (`~/views/calendar/calendar-
 * items.ts`, `~/views/home/today.ts`). today is midnight of the day the events are being read for.
 */
function toCalEvent(wire: WireCalendarEvent, today: number): CalEvent {
  const start = Date.parse(wire.start);
  const end = wire.end ? Date.parse(wire.end) : null;
  const sameDay = end !== null && !wire.allDay && startOfDay(end) === startOfDay(start);
  const dates = wire.allDay ? allDayOffsets(wire, today) : null;
  const dayOffset = dates ? dates.first : daysBetween(today, start);
  const lastDayOffset = dates ? dates.last : lastDayOffsetOf(start, end, dayOffset, today);
  return {
    id: wire.id,
    title: wire.title,
    time: wire.allDay ? "" : clock(start),
    ...(sameDay && end !== null ? { endTime: clock(end) } : {}),
    ...(wire.allDay ? { allDay: true } : {}),
    ...(wire.location ? { location: wire.location } : {}),
    ...(wire.url ? { url: wire.url } : {}),
    ...(wire.joinUrl ? { joinUrl: wire.joinUrl } : {}),
    ...(wire.calendar ? { calendar: wire.calendar } : {}),
    dayOffset,
    ...(lastDayOffset === undefined ? {} : { lastDayOffset }),
  };
}

/** Every event a calendar range answer carries, relative to today. */
export function toCalEvents(list: CalendarList, today: number): CalEvent[] {
  return list.events.map((event) => toCalEvent(event, today));
}

/** How reading Google Calendar went, as the screens read it. */
export function toCalGoogle(list: CalendarList): CalGoogle {
  return {
    known: true,
    connected: list.googleConnected,
    error: list.googleError,
    stale: list.googleStale,
  };
}
