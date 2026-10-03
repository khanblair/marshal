import type { CalEvent } from "~/mock";

/**
 * Whether a Google Calendar event is on a day: `weekday` is 0 for Sunday, `off` is the day's offset
 * from today. An event that runs over several days is on each of them, not only its first.
 */
export function coversDay(e: CalEvent, weekday: number, off: number): boolean {
  if (e.days?.includes(weekday)) return true;
  if (e.dayOffset === undefined) return false;
  return off >= e.dayOffset && off <= (e.lastDayOffset ?? e.dayOffset);
}

/** Whether a day is one after the event began, so its start time says nothing about that day. */
export function continuesOn(e: CalEvent, off: number): boolean {
  return e.dayOffset !== undefined && off > e.dayOffset;
}
