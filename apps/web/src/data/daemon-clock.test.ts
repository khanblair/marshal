import { describe, expect, it } from "vitest";
import { createDaemonClock } from "./daemon-clock";

const BASE = Date.UTC(2026, 8, 25, 10, 0, 0, 0);
const iso = (ms: number) => new Date(ms).toISOString();

/** A sample of an answer whose round trip took `trip` ms and whose daemon time was `ahead` ms ahead of the middle. */
function sample(sentAt: number, trip: number, ahead: number) {
  const receivedAt = sentAt + trip;
  return { serverTime: iso(sentAt + trip / 2 + ahead), sentAt, receivedAt };
}

describe("createDaemonClock", () => {
  it("has no offset before any answer", () => {
    const clock = createDaemonClock(() => BASE);
    expect(clock.offsetMs()).toBe(0);
    expect(clock.now()).toBe(BASE);
  });

  it("takes the daemon's time to be the middle of the round trip", () => {
    const clock = createDaemonClock(() => BASE + 1000);
    clock.observe(sample(BASE, 100, 5000));
    expect(clock.offsetMs()).toBe(5000);
    expect(clock.now()).toBe(BASE + 1000 + 5000);
  });

  it("follows a daemon whose clock is behind", () => {
    const clock = createDaemonClock(() => BASE);
    clock.observe(sample(BASE, 40, -2500));
    expect(clock.offsetMs()).toBe(-2500);
    expect(clock.now()).toBe(BASE - 2500);
  });

  it("prefers the fast answer, so one slow answer does not skew the offset", () => {
    const clock = createDaemonClock(() => BASE);
    clock.observe(sample(BASE, 20, 1000));
    // A slow answer whose middle is not where the daemon's time was taken: it reads 900 ms.
    clock.observe(sample(BASE + 1000, 2000, 900));
    expect(clock.offsetMs()).toBe(1000);
  });

  it("switches to a faster answer when one comes", () => {
    const clock = createDaemonClock(() => BASE);
    clock.observe(sample(BASE, 2000, 900));
    expect(clock.offsetMs()).toBe(900);
    clock.observe(sample(BASE + 5000, 30, 1000));
    expect(clock.offsetMs()).toBe(1000);
  });

  it("forgets samples older than a minute, counted from the newest answer", () => {
    const clock = createDaemonClock(() => BASE);
    clock.observe(sample(BASE, 10, 1000));
    // Well over a minute later, a slower answer with a different offset: the old fast one is gone.
    clock.observe(sample(BASE + 61_000, 500, 400));
    expect(clock.offsetMs()).toBe(400);
  });

  it("keeps its offset while nothing new arrives", () => {
    let local = BASE;
    const clock = createDaemonClock(() => local);
    clock.observe(sample(BASE, 10, 700));
    local += 10 * 60_000;
    expect(clock.offsetMs()).toBe(700);
    expect(clock.now()).toBe(local + 700);
  });

  it("uses the newest of two equally fast answers", () => {
    const clock = createDaemonClock(() => BASE);
    clock.observe(sample(BASE, 50, 100));
    clock.observe(sample(BASE + 1000, 50, 300));
    expect(clock.offsetMs()).toBe(300);
  });

  it("ignores a time it cannot read and an answer that arrived before it was sent", () => {
    const clock = createDaemonClock(() => BASE);
    clock.observe({ serverTime: "soon", sentAt: BASE, receivedAt: BASE + 10 });
    clock.observe({ serverTime: iso(BASE), sentAt: BASE + 10, receivedAt: BASE });
    expect(clock.offsetMs()).toBe(0);
  });

  it("defaults to the real clock", () => {
    const clock = createDaemonClock();
    expect(Math.abs(clock.now() - Date.now())).toBeLessThan(50);
  });
});
