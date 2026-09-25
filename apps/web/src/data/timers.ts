/** A timer id. It is whatever `setTimeout` returns here, so a fake can return anything. */
export type TimerId = ReturnType<typeof setTimeout>;

/** The two timer functions, so a test can hand in fakes and count what is left running. */
export interface Timers {
  setTimeout(callback: () => void, ms: number): TimerId;
  clearTimeout(id: TimerId): void;
}

/** The page's own timers, looked up at the moment of the call so fake timers in tests are used. */
export const realTimers: Timers = {
  setTimeout: (callback, ms) => globalThis.setTimeout(callback, ms),
  clearTimeout: (id) => globalThis.clearTimeout(id),
};
