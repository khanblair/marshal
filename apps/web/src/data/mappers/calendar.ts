import type { CalendarEvent as WireCalendarEvent, CalendarList } from "@marshal/protocol";

const DAY_MS = 86_400_000;

/** One Google Calendar event, as the Calendar view and Home's coming-up list draw it. */
export interface CalEvent {
  id: string;
  title: string;
  time: string;
  days?: number[];
  dayOffset?: number;
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
  return {
    id: wire.id,
    title: wire.title,
    time: clockOf(start),
    dayOffset: Math.round((startOfDay(start.getTime()) - today) / DAY_MS),
  };
}

/** Every event a calendar range answer carries, relative to today. */
export function toCalEvents(list: CalendarList, today: number): CalEvent[] {
  return list.events.map((event) => toCalEvent(event, today));
}
