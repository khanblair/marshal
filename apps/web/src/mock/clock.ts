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
 */
export function createClock(): Clock {
  const [tick, setTick] = createSignal(undefined, { equals: false });
  return {
    now: () => {
      tick();
      return Date.now();
    },
    pulse: () => setTick(),
  };
}
