import { createMemo, createRoot } from "solid-js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setZone, startOfDay } from "~/data/zone";
import type { Ctx } from "~/mock/context";
import { createTestContext } from "~/testing/test-store";
import { followToday } from "./today";

const HOUR = 3_600_000;
const SLACK = 1000;
const at = (iso: string): number => Date.parse(iso);

const followed: Array<() => void> = [];
beforeEach(() => {
  vi.useFakeTimers();
  // The browser queues a storage event on a timer for each write, which these tests count.
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => undefined);
  vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => undefined);
});
afterEach(() => {
  vi.restoreAllMocks();
  // A test that failed before it stopped must not leave a timer or an effect for the next one.
  for (const stop of followed.splice(0)) stop();
  vi.useRealTimers();
});

/** A store made at `iso` in `zone`, with today followed inside a root. */
function follow(iso: string, zone: string, onRoll?: () => void) {
  setZone(zone);
  vi.setSystemTime(at(iso));
  const ctx: Ctx = createTestContext();
  let stop = (): void => undefined;
  let dispose = (): void => undefined;
  createRoot((disposeRoot) => {
    dispose = disposeRoot;
    stop = followToday(ctx, onRoll);
  });
  const stopAll = (): void => {
    stop();
    dispose();
  };
  followed.push(stopAll);
  return { ctx, stop: stopAll };
}

describe("followToday", () => {
  it("starts on midnight of the day it is in the zone", () => {
    const { ctx, stop } = follow("2026-10-08T22:00:00Z", "UTC");
    expect(ctx.today).toBe(at("2026-10-08T00:00:00Z"));
    stop();
  });

  it("moves today at the next midnight, and at every midnight after", () => {
    const { ctx, stop } = follow("2026-10-08T22:00:00Z", "UTC");
    vi.advanceTimersByTime(2 * HOUR - 1);
    expect(ctx.today).toBe(at("2026-10-08T00:00:00Z"));
    vi.advanceTimersByTime(SLACK + 1);
    expect(ctx.today).toBe(at("2026-10-09T00:00:00Z"));
    vi.advanceTimersByTime(24 * HOUR);
    expect(ctx.today).toBe(at("2026-10-10T00:00:00Z"));
    stop();
  });

  it("rolls at the midnight of the zone, not of the device", () => {
    // 21:00 UTC is midnight in Kampala.
    const { ctx, stop } = follow("2026-10-08T20:00:00Z", "Africa/Kampala");
    expect(ctx.today).toBe(at("2026-10-07T21:00:00Z"));
    vi.advanceTimersByTime(HOUR + SLACK);
    expect(ctx.today).toBe(at("2026-10-08T21:00:00Z"));
    stop();
  });

  it("counts a 23 hour day as one day", () => {
    // 00:30 on the day Los Angeles puts its clocks forward: midnight is 22 and a half hours away.
    const { ctx, stop } = follow("2026-03-08T08:30:00Z", "America/Los_Angeles");
    vi.advanceTimersByTime(22.5 * HOUR + SLACK);
    expect(ctx.today).toBe(at("2026-03-09T07:00:00Z"));
    stop();
  });

  it("moves today when the zone changes, and sets the next midnight again", () => {
    const { ctx, stop } = follow("2026-10-08T22:00:00Z", "UTC");
    setZone("Africa/Kampala");
    expect(ctx.today).toBe(at("2026-10-08T21:00:00Z"));
    // The next midnight is now in 23 hours, not 2, and only one timer is waiting for it.
    expect(vi.getTimerCount()).toBe(1);
    vi.advanceTimersByTime(23 * HOUR - 1);
    expect(ctx.today).toBe(at("2026-10-08T21:00:00Z"));
    vi.advanceTimersByTime(SLACK + 1);
    expect(ctx.today).toBe(at("2026-10-09T21:00:00Z"));
    stop();
  });

  it("says so each time today moved, and not when it did not", () => {
    const rolled = vi.fn();
    const { stop } = follow("2026-10-08T22:00:00Z", "UTC", rolled);
    expect(rolled).not.toHaveBeenCalled();
    vi.advanceTimersByTime(2 * HOUR + SLACK);
    expect(rolled).toHaveBeenCalledTimes(1);
    setZone("UTC");
    expect(rolled).toHaveBeenCalledTimes(1);
    setZone("Africa/Kampala");
    expect(rolled).toHaveBeenCalledTimes(2);
    stop();
  });

  it("keeps the calendar on today when it showed today, and leaves it where it was otherwise", () => {
    const { ctx, stop } = follow("2026-10-08T22:00:00Z", "UTC");
    expect(ctx.S.calCursor).toBe(ctx.today);
    vi.advanceTimersByTime(2 * HOUR + SLACK);
    expect(ctx.S.calCursor).toBe(at("2026-10-09T00:00:00Z"));
    ctx.S.calCursor = at("2026-11-02T00:00:00Z");
    vi.advanceTimersByTime(24 * HOUR);
    expect(ctx.today).toBe(at("2026-10-10T00:00:00Z"));
    expect(ctx.S.calCursor).toBe(at("2026-11-02T00:00:00Z"));
    stop();
  });

  it("can be read in a memo, which updates when today moves", () => {
    const { ctx, stop } = follow("2026-10-08T22:00:00Z", "UTC");
    createRoot((dispose) => {
      const today = createMemo(() => ctx.today);
      expect(today()).toBe(at("2026-10-08T00:00:00Z"));
      vi.advanceTimersByTime(2 * HOUR + SLACK);
      expect(today()).toBe(at("2026-10-09T00:00:00Z"));
      dispose();
    });
    stop();
  });

  it("stops: no timer is left, and today stays", () => {
    const { ctx, stop } = follow("2026-10-08T22:00:00Z", "UTC");
    stop();
    expect(vi.getTimerCount()).toBe(0);
    vi.advanceTimersByTime(30 * HOUR);
    expect(ctx.today).toBe(startOfDay(at("2026-10-08T22:00:00Z"), "UTC"));
  });
});
