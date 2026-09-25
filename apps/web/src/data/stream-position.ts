import type { Resync, Event as WireEvent } from "@marshal/protocol";

export interface Accepted {
  /** True when the events came from another epoch, which means the daemon started again. */
  restarted: boolean;
  /** The events that are new, in order. */
  fresh: WireEvent[];
}

/**
 * Where the client left off: the epoch and the number of the last event it applied. The number
 * only grows within an epoch, and a client that follows some topics sees gaps in it, so it is used
 * to skip repeats and never to look for what is missing.
 */
export class StreamPosition {
  epoch = "";
  seq = 0;

  /** Keeps the events with a number above the last one applied, and moves the position past them. */
  accept(epoch: string, events: readonly WireEvent[]): Accepted {
    const restarted = epoch !== this.epoch;
    if (restarted) {
      // The numbers of a new epoch begin again, so the old position must not filter them out.
      this.epoch = epoch;
      this.seq = 0;
    }
    const fresh: WireEvent[] = [];
    for (const event of events) {
      if (event.seq <= this.seq) continue;
      this.seq = event.seq;
      fresh.push(event);
    }
    return { restarted, fresh };
  }

  /** Moves to the position the daemon names. Events after it follow on the same connection. */
  resync(frame: Resync): void {
    this.epoch = frame.epoch;
    this.seq = frame.seq;
  }
}
