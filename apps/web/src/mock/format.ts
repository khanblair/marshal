import { DAY_MS, HOUR_MS, MINUTE_MS } from "./constants";

const CENTS_PER_DOLLAR = 100;
/** Anything newer than this reads as "Just now". */
const JUST_NOW_MS = 45_000;

export const money = (v: number): string =>
  `$${(Math.round(v * CENTS_PER_DOLLAR) / CENTS_PER_DOLLAR).toFixed(2)}`;

/** Relative time of `ts` as seen at `now`, in the prototype's words. */
export function relTime(ts: number, now: number): string {
  const d = now - ts;
  if (d < JUST_NOW_MS) return "Just now";
  if (d < HOUR_MS) return `${Math.round(d / MINUTE_MS)} min ago`;
  if (d < DAY_MS) return `${Math.round(d / HOUR_MS)} h ago`;
  const n = Math.round(d / DAY_MS);
  return n === 1 ? "Yesterday" : `${n} days ago`;
}

export const full = (ts: number): string =>
  new Date(ts).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
