import { createSignal } from "solid-js";
import { currentTimeZone } from "./time-zones";

/*
 * The one time zone every date and time on screen uses: the profile's, or this device's when the
 * profile has none. Each function takes the zone last so a test can pass one; left out, it is the
 * chosen zone, read from a signal so a memo that shows a time follows a change.
 */

const SECOND_MS = 1000;
const DAY_MS = 86_400_000;
const HOURS_PER_DAY = 24;
const TWO_DIGITS = 2;
const FOUR_DIGITS = 4;

/** Where the zone is kept on this device, so the first screen of a visit already uses it. */
const REMEMBERED_ZONE_KEY = "marshal.zone";

function rememberedZone(): string {
  try {
    return window.localStorage.getItem(REMEMBERED_ZONE_KEY) ?? "";
  } catch {
    return "";
  }
}

function remember(zone: string): void {
  try {
    if (zone === "") window.localStorage.removeItem(REMEMBERED_ZONE_KEY);
    else window.localStorage.setItem(REMEMBERED_ZONE_KEY, zone);
  } catch {
    // A page with no storage still works: the profile says the zone once it loads.
  }
}

// What the profile said last time, because the profile only arrives after the first snapshots, and
// the cards read in them count their dates from today in this zone.
const [chosen, setChosen] = createSignal(known(rememberedZone()) ? rememberedZone() : "");

/** True when the browser knows the zone name. */
function known(zone: string): boolean {
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: zone });
    return true;
  } catch {
    return false;
  }
}

/** Chooses the zone to use. An empty or unknown name means the device's own zone. */
export function setZone(name: string): void {
  const next = name.trim();
  const zone = next !== "" && known(next) ? next : "";
  setChosen(zone);
  remember(zone);
}

/** The zone in use, such as "Africa/Kampala": the chosen one, or the device's. */
export const zoneName = (): string => chosen() || currentTimeZone() || "UTC";

/** A moment read on the wall clock of a zone. `month` counts from 1. */
interface Wall {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
}

const wallFormats = new Map<string, Intl.DateTimeFormat>();

function wallFormat(zone: string): Intl.DateTimeFormat {
  let format = wallFormats.get(zone);
  if (!format) {
    format = new Intl.DateTimeFormat("en-US", {
      timeZone: zone,
      hourCycle: "h23",
      numberingSystem: "latn",
      year: "numeric",
      month: "numeric",
      day: "numeric",
      hour: "numeric",
      minute: "numeric",
      second: "numeric",
    });
    wallFormats.set(zone, format);
  }
  return format;
}

function wallOf(ms: number, zone: string): Wall {
  const wall: Wall = { year: 0, month: 1, day: 1, hour: 0, minute: 0, second: 0 };
  for (const part of wallFormat(zone).formatToParts(ms)) {
    if (part.type in wall) wall[part.type as keyof Wall] = Number(part.value);
  }
  wall.hour %= HOURS_PER_DAY;
  return wall;
}

/** The wall clock as if it were UTC, so plain arithmetic on it never meets a clock change. */
const utcOf = (wall: Wall): number =>
  Date.UTC(wall.year, wall.month - 1, wall.day, wall.hour, wall.minute, wall.second);

/** How far the zone's wall clock is ahead of UTC at a moment. */
const offsetAt = (ms: number, zone: string): number =>
  utcOf(wallOf(ms, zone)) - Math.floor(ms / SECOND_MS) * SECOND_MS;

/**
 * The moment a wall clock (given as if UTC) reads in a zone. The offsets a day either side catch a
 * clock change. A clock read twice takes the first; one that was skipped takes the offset before.
 */
function fromWall(wall: number, zone: string): number {
  const before = wall - offsetAt(wall - DAY_MS, zone);
  const after = wall - offsetAt(wall + DAY_MS, zone);
  const fits = [before, after].filter((at) => at + offsetAt(at, zone) === wall);
  return fits.length > 0 ? Math.min(...fits) : before;
}

const pad = (n: number, width = TWO_DIGITS): string => String(n).padStart(width, "0");

/** The first moment of the day `ms` falls on in the zone, as epoch ms. Right across clock changes. */
export function startOfDay(ms: number, zone: string = zoneName()): number {
  const wall = wallOf(ms, zone);
  return fromWall(Date.UTC(wall.year, wall.month - 1, wall.day), zone);
}

/** The first moment of the month `ms` falls on in the zone. */
export function startOfMonth(ms: number, zone: string = zoneName()): number {
  const wall = wallOf(ms, zone);
  return fromWall(Date.UTC(wall.year, wall.month - 1, 1), zone);
}

/** The same wall clock `n` days later (or earlier): a day with a clock change is not 24 hours. */
export function addDays(ms: number, n: number, zone: string = zoneName()): number {
  if (n === 0) return ms;
  return fromWall(ms + offsetAt(ms, zone) + n * DAY_MS, zone);
}

/** The same wall clock `n` months later, as `Date#setMonth` moves it (the 31st rolls over). */
export function addMonths(ms: number, n: number, zone: string = zoneName()): number {
  const wall = wallOf(ms, zone);
  const sub = ms - Math.floor(ms / SECOND_MS) * SECOND_MS;
  const moved = Date.UTC(
    wall.year,
    wall.month - 1 + n,
    wall.day,
    wall.hour,
    wall.minute,
    wall.second,
  );
  return fromWall(moved + sub, zone);
}

/** How many days the month `ms` falls on has. */
export function daysInMonth(ms: number, zone: string = zoneName()): number {
  const wall = wallOf(ms, zone);
  return new Date(Date.UTC(wall.year, wall.month, 0)).getUTCDate();
}

/** The weekday in the zone, 0 for Sunday. */
export function dayOfWeek(ms: number, zone: string = zoneName()): number {
  const wall = wallOf(ms, zone);
  return new Date(Date.UTC(wall.year, wall.month - 1, wall.day)).getUTCDay();
}

/** The day of the month in the zone, 1 to 31. */
export const dayNumber = (ms: number, zone: string = zoneName()): number => wallOf(ms, zone).day;

/** The month in the zone, 1 to 12. */
export const monthNumber = (ms: number, zone: string = zoneName()): number =>
  wallOf(ms, zone).month;

/** The year in the zone. */
export const yearNumber = (ms: number, zone: string = zoneName()): number => wallOf(ms, zone).year;

/** The time of day in the zone on a 24 hour clock, "09:30". */
export function clock(ms: number, zone: string = zoneName()): string {
  const wall = wallOf(ms, zone);
  return `${pad(wall.hour)}:${pad(wall.minute)}`;
}

/** The date in the zone as "2026-10-08". */
export function dateKey(ms: number, zone: string = zoneName()): string {
  const wall = wallOf(ms, zone);
  return `${pad(wall.year, FOUR_DIGITS)}-${pad(wall.month)}-${pad(wall.day)}`;
}

/** How many calendar days `toMs` is after `fromMs` in the zone. Negative when it is before. */
export function daysBetween(fromMs: number, toMs: number, zone: string = zoneName()): number {
  const from = wallOf(fromMs, zone);
  const to = wallOf(toMs, zone);
  const days = (wall: Wall): number => Date.UTC(wall.year, wall.month - 1, wall.day);
  return Math.round((days(to) - days(from)) / DAY_MS);
}

const DATE_KEY = /^(\d{4})-(\d{2})-(\d{2})/;

/** The date a "2026-10-08" key names, as a day count, or null when it is not one. */
function keyDays(key: string): number | null {
  const match = DATE_KEY.exec(key);
  if (!match) return null;
  return Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3])) / DAY_MS;
}

/** How many days `to` is after `from`, both "2026-10-08" dates that belong to no zone. Null if not dates. */
export function daysBetweenDates(from: string, to: string): number | null {
  const a = keyDays(from);
  const b = keyDays(to);
  return a === null || b === null ? null : Math.round(b - a);
}

const displayFormats = new Map<string, Intl.DateTimeFormat>();

/** `Intl.DateTimeFormat` options in the zone, in the person's language. */
export function formatDate(
  ms: number,
  options: Intl.DateTimeFormatOptions,
  zone: string = zoneName(),
): string {
  if (!Number.isFinite(ms)) return "Invalid Date";
  const key = `${zone}|${JSON.stringify(options)}`;
  let format = displayFormats.get(key);
  if (!format) {
    format = new Intl.DateTimeFormat(undefined, { ...options, timeZone: zone });
    displayFormats.set(key, format);
  }
  return format.format(ms);
}

/** "Oct 8, 2026, 9:30 AM". */
export const formatDateTime = (ms: number, zone: string = zoneName()): string =>
  formatDate(ms, { dateStyle: "medium", timeStyle: "short" }, zone);

/** "Oct 8". */
export const formatShortDate = (ms: number, zone: string = zoneName()): string =>
  formatDate(ms, { month: "short", day: "numeric" }, zone);

const labelFormats = new Map<string, Intl.DateTimeFormat>();

/** The short name of the zone at a moment, such as "EAT" or "PDT" (or "GMT+3" where none is known). */
export function zoneLabel(ms: number = Date.now(), zone: string = zoneName()): string {
  let format = labelFormats.get(zone);
  if (!format) {
    format = new Intl.DateTimeFormat(undefined, { timeZone: zone, timeZoneName: "short" });
    labelFormats.set(zone, format);
  }
  return format.formatToParts(ms).find((part) => part.type === "timeZoneName")?.value ?? zone;
}
