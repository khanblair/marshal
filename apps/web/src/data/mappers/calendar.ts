import type { CalendarList, CalendarEvent as WireCalendarEvent } from "@marshal/protocol";

const DAY_MS = 86_400_000;

/** One Google Calendar event, as the Calendar view and Home's coming-up list draw it. */
export interface CalEvent {
  id: string;
  title: string;
  /** `09:30`, the viewer's own clock. Empty for an all-day event, which has no time of day. */
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

/** Midnight of the day ms falls on, in the viewer's own time zone. */
function startOfDay(ms: number): number {
  const date = new Date(ms);
  date.setHours(0, 0, 0, 0);
  return date.getTime();
}

/** "09:30", the viewer's own local clock. */
function clockOf(date: Date): string {
  const hours = String(date.getHours()).padStart(2, "0");
  const minutes = String(date.getMinutes()).padStart(2, "0");
  return `${hours}:${minutes}`;
}

/**
 * One event, with its wire `start` (an absolute moment) turned into `time` and `dayOffset` - the
 * relative shape every screen that draws a calendar already reads (`~/views/calendar/calendar-
 * items.ts`, `~/views/home/today.ts`). today is midnight of the day the events are being read for.
 */
function toCalEvent(wire: WireCalendarEvent, today: number): CalEvent {
  const start = new Date(wire.start);
  const end = wire.end ? new Date(wire.end) : null;
  const sameDay =
    end !== null && !wire.allDay && startOfDay(end.getTime()) === startOfDay(start.getTime());
  return {
    id: wire.id,
    title: wire.title,
    time: wire.allDay ? "" : clockOf(start),
    ...(sameDay && end ? { endTime: clockOf(end) } : {}),
    ...(wire.allDay ? { allDay: true } : {}),
    ...(wire.location ? { location: wire.location } : {}),
    ...(wire.url ? { url: wire.url } : {}),
    ...(wire.joinUrl ? { joinUrl: wire.joinUrl } : {}),
    ...(wire.calendar ? { calendar: wire.calendar } : {}),
    dayOffset: Math.round((startOfDay(start.getTime()) - today) / DAY_MS),
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
