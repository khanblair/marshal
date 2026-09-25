import { createSignal } from "solid-js";

export interface Clock {
  /** Current time in ms. Reading it inside a memo or effect re-runs it on every tick. */
  now(): number;
  /** Called by the simulation tick once a second. */
  pulse(): void;
}

/**
 * The prototype re-rendered every second from its tick and read `Date.now()` while
 * rendering. `now()` does the same: it subscribes to the tick and returns the wall clock.
 * With `#nosim` nothing pulses, so time-based text only changes when something else
 * re-renders it, and a frozen test clock gives the same text as the prototype.
 *
 * `offsetMs` is how far the daemon's clock is ahead of this one, so "4 min ago" counts from the
 * daemon's time when the two differ. It is 0 without a daemon.
 */
export function createClock(offsetMs: () => number = () => 0): Clock {
  const [tick, setTick] = createSignal(undefined, { equals: false });
  return {
    now: () => {
      tick();
      return Date.now() + offsetMs();
    },
    pulse: () => setTick(),
  };
}
