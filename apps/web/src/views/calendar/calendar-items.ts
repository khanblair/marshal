/**
 * What the calendar shows on a day: schedule runs, Google Calendar events, and due cards,
 * with the text of their tooltips and confirm dialogs. Port of `itemsFor` in
 * design/CalendarView.dc.html. Pure: the view passes the store's lists in.
 */
import type { CalEvent, Card, Schedule } from "~/mock";
import { fullDate } from "./calendar-dates";

/** Which entry an item came from, so the view knows what a click opens. */
type CalTarget =
  | { kind: "schedule"; schedule: Schedule }
  | { kind: "event"; event: CalEvent; day: number }
  | { kind: "card"; card: Card };

export interface CalItem {
  /** `brief` or `job` for schedules, `event` for Google Calendar, `due` for cards. */
  kind: Schedule["kind"] | "event" | "due";
  icon: string;
  /** "07:00", or empty for a due card. */
  time: string;
  label: string;
  tip: string;
  target: CalTarget;
}

export interface CalSource {
  schedules: readonly Schedule[];
  events: readonly CalEvent[];
  /** The project's cards. */
  cards: readonly Card[];
  projectName: string;
  /** Midnight of today. */
  today: number;
  dayMs: number;
}

export interface ConfirmText {
  title: string;
  message: string;
  action: string;
}

/** An interval job runs on the three days from today, every 30 minutes. */
const INTERVAL_DAYS = 3;
const ALL_PROJECTS = "All projects";
/** Sorts an item without a time after every timed one. */
const NO_TIME_SORT_KEY = "99";

const dayOffset = (t: number, today: number, dayMs: number): number =>
  Math.round((t - today) / dayMs);

function runsOn(s: Schedule, dow: number, off: number): boolean {
  if (s.trigger === "Interval") return off >= 0 && off < INTERVAL_DAYS;
  return s.days.includes(dow);
}

function scheduleItems(src: CalSource, dow: number, off: number): CalItem[] {
  return src.schedules
    .filter((s) => s.enabled && (s.project === ALL_PROJECTS || s.project === src.projectName))
    .filter((s) => runsOn(s, dow, off))
    .map((s) => ({
      kind: s.kind,
      icon: s.icon,
      time: s.time,
      label: s.trigger === "Interval" ? `${s.name}, every 30 min` : s.name,
      tip: `${s.name}. ${s.when}. ${s.action}`,
      target: { kind: "schedule", schedule: s },
    }));
}

function eventItems(src: CalSource, t: number, dow: number, off: number): CalItem[] {
  return src.events
    .filter((e) => e.days?.includes(dow) || e.dayOffset === off)
    .map((e) => ({
      kind: "event",
      icon: "calendar",
      time: e.time,
      label: e.title,
      tip: `${e.title} at ${e.time}, from Google Calendar`,
      target: { kind: "event", event: e, day: t },
    }));
}

function dueItems(src: CalSource, off: number): CalItem[] {
  return src.cards
    .filter((c) => c.due === off)
    .map((c) => ({
      kind: "due",
      icon: "calendar-check",
      time: "",
      label: `#${c.id} ${c.title}`,
      tip: `#${c.id} ${c.title} is due`,
      target: { kind: "card", card: c },
    }));
}

/** Everything on the day starting at midnight `t`, earliest time first, due cards last. */
export function itemsForDay(t: number, src: CalSource): CalItem[] {
  const dow = new Date(t).getDay();
  const off = dayOffset(t, src.today, src.dayMs);
  return [
    ...scheduleItems(src, dow, off),
    ...eventItems(src, t, dow, off),
    ...dueItems(src, off),
  ].sort((a, b) => (a.time || NO_TIME_SORT_KEY).localeCompare(b.time || NO_TIME_SORT_KEY));
}

/** Confirm dialog of a schedule: what it does and how it treats missed runs. */
export function scheduleDialog(s: Schedule): ConfirmText {
  return {
    title: s.name,
    message: `${s.when}. ${s.action}. Missed runs: ${s.missed.toLowerCase()}.`,
    action: "Edit schedule",
  };
}

/** Confirm dialog of a Google Calendar event on day `t`. */
export function eventDialog(e: CalEvent, t: number): ConfirmText {
  return {
    title: e.title,
    message: `${e.time} on ${fullDate(t)}. Synced from Google Calendar. Briefs use it for smart timing.`,
    action: "Close",
  };
}
