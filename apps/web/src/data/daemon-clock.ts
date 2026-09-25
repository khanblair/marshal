import type { Timestamp } from "@marshal/protocol";

/** One answer seen from the client: what the daemon's clock said, and when we asked and heard. */
interface ClockSample {
  serverTime: Timestamp;
  /** The local time, in ms, when the request was sent. */
  sentAt: number;
  /** The local time, in ms, when the answer arrived. */
  receivedAt: number;
}

export interface DaemonClock {
  /** Learns from one answer. A sample that makes no sense (an unreadable time) is ignored. */
  observe(sample: ClockSample): void;
  /** The daemon's current time in ms, as far as it is known: the local clock plus the offset. */
  now(): number;
  /** How far the daemon's clock is ahead of this one, in ms. Negative when it is behind. 0 before any answer. */
  offsetMs(): number;
}

/** Only the samples from the last minute count, so a change of the local clock is followed. */
const WINDOW_MS = 60_000;

interface Measured {
  roundTripMs: number;
  offsetMs: number;
  receivedAt: number;
}

/**
 * The offset comes from the sample with the smallest round trip in the last minute, because the
 * time an answer spent on the way makes its estimate worse. The daemon's time is taken to be
 * the middle of the round trip. The window is counted from the newest answer, so an idle app
 * keeps its offset instead of going back to zero.
 */
export function createDaemonClock(localNow: () => number = Date.now): DaemonClock {
  let samples: Measured[] = [];
  let best: Measured | null = null;

  return {
    observe({ serverTime, sentAt, receivedAt }) {
      const roundTripMs = receivedAt - sentAt;
      const server = Date.parse(serverTime);
      if (roundTripMs < 0 || Number.isNaN(server)) return;
      samples.push({ roundTripMs, offsetMs: server - (sentAt + receivedAt) / 2, receivedAt });
      samples = samples.filter((s) => s.receivedAt > receivedAt - WINDOW_MS);
      best = samples.reduce((a, b) => (b.roundTripMs <= a.roundTripMs ? b : a));
    },
    now: () => localNow() + (best?.offsetMs ?? 0),
    offsetMs: () => best?.offsetMs ?? 0,
  };
}
