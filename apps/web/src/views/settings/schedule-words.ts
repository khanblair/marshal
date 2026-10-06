import type { ScheduleChannel, ScheduleSection } from "@marshal/protocol";

/*
 * The words a schedule's time is written in. The editor's time and day pickers write `when`, `time`,
 * and `days` together from these, so the sentence a person reads and the cron the daemon runs cannot
 * drift apart. The daemon reads the sentence back the same way (schedules.Reconcile).
 */

const MONDAY = 1;
const FRIDAY = 5;
const SATURDAY = 6;
const SUNDAY = 0;
const WEEK_LENGTH = 7;

/** The days as the daemon counts them (Sunday is 0, Monday is 1), in the order a week reads. */
export const DAYS = [
  { value: 1, short: "Mon", long: "Monday" },
  { value: 2, short: "Tue", long: "Tuesday" },
  { value: 3, short: "Wed", long: "Wednesday" },
  { value: 4, short: "Thu", long: "Thursday" },
  { value: 5, short: "Fri", long: "Friday" },
  { value: 6, short: "Sat", long: "Saturday" },
  { value: 0, short: "Sun", long: "Sunday" },
] as const;

export const WEEKDAYS: readonly number[] = Array.from(
  { length: FRIDAY - MONDAY + 1 },
  (_, i) => MONDAY + i,
);
export const WEEKEND: readonly number[] = [SUNDAY, SATURDAY];

/** "08:00" as people say it, "8:00". A time that is not H:MM is left as it is. */
export function clockWords(time: string): string {
  const match = /^(\d{1,2}):(\d{2})$/.exec(time.trim());
  return match ? `${Number(match[1])}:${match[2]}` : time.trim();
}

/** The days that are set, each once, in the order a week reads. Every day set means none is listed. */
export function normalDays(days: readonly number[]): number[] {
  const set = new Set(days.map((day) => day % WEEK_LENGTH));
  const listed = DAYS.map((day) => day.value).filter((value) => set.has(value));
  return listed.length === WEEK_LENGTH ? [] : listed;
}

const sameDays = (a: readonly number[], b: readonly number[]): boolean =>
  a.length === b.length && b.every((day) => a.includes(day));

/** "Every weekday at 8:00", "Every Monday at 9:00", "Every Mon, Wed and Fri at 8:15", "Every day at 6:00". */
export function whenForDays(days: readonly number[], time: string): string {
  const chosen = normalDays(days);
  const at = `at ${clockWords(time)}`;
  if (chosen.length === 0) return `Every day ${at}`;
  if (sameDays(chosen, WEEKDAYS)) return `Every weekday ${at}`;
  if (sameDays(chosen, WEEKEND)) return `Every weekend ${at}`;
  const named = DAYS.filter((day) => chosen.includes(day.value));
  if (named.length === 1) return `Every ${named[0]?.long} ${at}`;
  const short = named.map((day) => day.short);
  const last = short.pop();
  return `Every ${short.join(", ")} and ${last} ${at}`;
}

export type IntervalUnit = "minutes" | "hours";

/** "Every 30 minutes", "Every 4 hours", and "Every minute" is never written: one is "Every 1 minutes". */
export function whenForInterval(every: number, unit: IntervalUnit): string {
  return `Every ${every} ${unit}`;
}

/** The number and unit in "Every 4 hours", or null for words that are not an interval. */
export function readInterval(when: string): { every: number; unit: IntervalUnit } | null {
  const match = /^every\s+(\d+)\s+(minute|hour)s?\b/i.exec(when.trim());
  if (!match?.[1] || !match[2]) return null;
  return { every: Number(match[1]), unit: match[2].toLowerCase() === "hour" ? "hours" : "minutes" };
}

/** One sentence for a brief: its parts, and where it goes. */
export function briefAction(
  sections: readonly string[],
  deliver: readonly string[],
  catalog: { sections: readonly ScheduleSection[]; channels: readonly ScheduleChannel[] },
): string {
  const label = <T extends { id: string; label: string }>(list: readonly T[], id: string): string =>
    list.find((one) => one.id === id)?.label ?? id;
  const parts = sections.map((id) => label(catalog.sections, id)).join(", ");
  const chats = deliver.map((id) => label(catalog.channels, id));
  const where = chats.length === 0 ? "kept in History" : `sent to ${chats.join(" and ")}`;
  return `${parts || "An empty brief"}, ${where}`;
}

/** The parts to list: the ones already in the brief first, in their order, then the rest as the catalog has them. */
export function orderedSections(
  selected: readonly string[],
  all: readonly ScheduleSection[],
): ScheduleSection[] {
  const chosen = selected
    .map((id) => all.find((section) => section.id === id))
    .filter((section): section is ScheduleSection => section !== undefined);
  return [...chosen, ...all.filter((section) => !selected.includes(section.id))];
}

/** A list with the id added at the end, or taken out when it is there. */
export function toggled(list: readonly string[], id: string): string[] {
  return list.includes(id) ? list.filter((one) => one !== id) : [...list, id];
}
