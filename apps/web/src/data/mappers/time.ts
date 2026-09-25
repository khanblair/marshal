import type { Timestamp } from "@marshal/protocol";
// The mock imports this from here, never the other way: the mock is deleted in Phase 13.
import { relTime } from "~/data/format";

/**
 * Turns a wire timestamp into the milliseconds since 1970 that the screens keep (`Card.upd`,
 * `Chat.last`, and the rest). The daemon never sends a time it cannot write, so a value that does
 * not parse is a bug on the daemon's side, and this throws instead of showing "NaN days ago".
 */
export function toMillis(ts: Timestamp): number {
  const ms = Date.parse(ts);
  if (Number.isNaN(ms)) throw new RangeError(`"${ts}" is not a timestamp`);
  return ms;
}

/**
 * The words for how long ago a wire timestamp was, such as "4 min ago", with the daemon's clock
 * as "now". It is what `M.rel` shows for the same time, so a screen does not get a second format.
 */
export function toAgo(ts: Timestamp, clock: { now(): number }): string {
  return relTime(toMillis(ts), clock.now());
}
