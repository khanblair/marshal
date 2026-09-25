/* Small pure formatters that the data layer needs and the screens use. They live here, not in
   `mock/`, because the mock is deleted in Phase 13 and a real module must never depend on it. */

const MINUTE_MS = 60_000;
const HOUR_MS = 3_600_000;
const DAY_MS = 86_400_000;

/** Anything newer than this reads as "Just now". */
const JUST_NOW_MS = 45_000;

/** Relative time of `ts` as seen at `now`, in the prototype's words. */
export function relTime(ts: number, now: number): string {
  const d = now - ts;
  if (d < JUST_NOW_MS) return "Just now";
  if (d < HOUR_MS) return `${Math.round(d / MINUTE_MS)} min ago`;
  if (d < DAY_MS) return `${Math.round(d / HOUR_MS)} h ago`;
  const n = Math.round(d / DAY_MS);
  return n === 1 ? "Yesterday" : `${n} days ago`;
}
