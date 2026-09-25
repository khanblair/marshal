import type { ApiClient } from "./api-client";
import { ApiError, clientError } from "./api-error";
import { type BackoffPolicy, backoffDelay } from "./backoff";
import type { EventStream, StreamDetail, StreamState } from "./event-stream";
import { realTimers, type TimerId, type Timers } from "./timers";
import type { TokenStore } from "./token";

/**
 * Where the app stands with the daemon. `starting` is before the first check. `reconnecting` is
 * a live connection that dropped and is being tried again by the stream.
 */
export type ConnectionState =
  | "starting"
  | "online"
  | "reconnecting"
  | "unreachable"
  | "unauthorized";

interface ConnectionSnapshot {
  state: ConnectionState;
  /** Why the last check failed, or null while things work. */
  lastError: ApiError | null;
  /** When the next check happens, in ms on the local clock, or null when none is planned. */
  retryAt: number | null;
}

export interface MachineDeps {
  api: Pick<ApiClient, "health" | "whoami">;
  tokens: Pick<TokenStore, "load" | "subscribe">;
  stream: Pick<EventStream, "start" | "stop">;
  /** Calls the function when the page comes back to the front or the network returns. */
  watchPage?: (onReturn: () => void) => () => void;
  timers?: Timers;
  /** The local clock in ms, for `retryAt`. */
  now?: () => number;
  /** Called with the whole picture each time any part of it changes. */
  onChange: (snapshot: ConnectionSnapshot) => void;
}

export interface ConnectionMachine {
  /** Runs the first check. Calling it again does nothing. */
  start(): void;
  /** Clears every timer, stops the stream, and ignores whatever is still on its way. */
  stop(): void;
  /** Checks now, and starts the waits again from a second. */
  retryNow(): void;
  /** The event stream reports its state here. */
  streamState(state: StreamState, detail: StreamDetail): void;
  /** The API client reports every 401 here. */
  reportUnauthorized(error: ApiError): void;
  /** Runs the function each time the app comes back online, except the first time it gets there. */
  onReconnected(listener: () => void): () => void;
}

type Outcome = { kind: "ok" } | { kind: "unauthorized" | "unreachable"; error: ApiError };

/** A check that hears nothing for this long counts as no answer. */
const CHECK_TIMEOUT_MS = 5000;
/** A stream that has not come back after this long is checked, to tell an unreachable daemon from a refused token. */
const RECONNECT_CHECK_MS = 15_000;
/** ...or after this many failed tries, whichever comes first. */
const RECONNECT_CHECK_ATTEMPTS = 3;
/** 1 second, then 2, then 4, and so on up to 10. */
const CHECK_RETRY: BackoffPolicy = { baseMs: 1000, capMs: 10_000, jitter: 0 };

class Machine implements ConnectionMachine {
  private state: ConnectionState = "starting";
  private lastError: ApiError | null = null;
  private retryAt: number | null = null;
  private started = false;
  private everOnline = false;
  private checking = false;
  private checkId = 0;
  private failures = 0;
  private abort: AbortController | null = null;
  private retryTimer: TimerId | null = null;
  private reconnectTimer: TimerId | null = null;
  private readonly cleanups: (() => void)[] = [];
  private readonly reconnected = new Set<() => void>();
  private readonly timers: Timers;
  private readonly now: () => number;

  constructor(private readonly deps: MachineDeps) {
    this.timers = deps.timers ?? realTimers;
    this.now = deps.now ?? Date.now;
  }

  start(): void {
    if (this.started) return;
    this.started = true;
    this.cleanups.push(
      this.deps.tokens.subscribe(() => this.tokenChanged()),
      this.deps.watchPage?.(() => this.retryNow()) ?? (() => undefined),
    );
    void this.check();
  }

  stop(): void {
    this.started = false;
    this.checkId += 1;
    this.checking = false;
    this.abort?.abort();
    this.abort = null;
    this.clearTimers();
    for (const cleanup of this.cleanups.splice(0)) cleanup();
    this.deps.stream.stop();
  }

  retryNow(): void {
    if (!this.started) return;
    this.failures = 0;
    void this.check();
  }

  onReconnected(listener: () => void): () => void {
    this.reconnected.add(listener);
    return () => {
      this.reconnected.delete(listener);
    };
  }

  reportUnauthorized(error: ApiError): void {
    // A check that is running reads its own 401, and a second report changes nothing.
    if (!this.started || this.checking || this.state === "unauthorized") return;
    this.deps.stream.stop();
    this.clearTimers();
    this.commit({ state: "unauthorized", lastError: error, retryAt: null });
    // One more look with a fresh token: in dev, a reset data folder gives the daemon a new one.
    void this.check();
  }

  streamState(state: StreamState, detail: StreamDetail): void {
    if (!this.started) return;
    if (state === "open" && this.state === "reconnecting") {
      this.clearReconnectTimer();
      this.commit({ state: "online", lastError: null, retryAt: null });
      this.notifyReconnected();
    } else if (state === "waiting" && this.state === "online") {
      this.commit({ state: "reconnecting", lastError: null, retryAt: null });
      this.armReconnectTimer();
    }
    if (
      state === "waiting" &&
      this.state === "reconnecting" &&
      detail.attempts === RECONNECT_CHECK_ATTEMPTS
    ) {
      void this.check();
    }
  }

  private tokenChanged(): void {
    if (this.state === "unauthorized" && !this.checking) void this.check();
  }

  private commit(next: ConnectionSnapshot): void {
    this.state = next.state;
    this.lastError = next.lastError;
    this.retryAt = next.retryAt;
    this.deps.onChange({ ...next });
  }

  private clearRetryTimer(): void {
    if (this.retryTimer !== null) this.timers.clearTimeout(this.retryTimer);
    this.retryTimer = null;
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== null) this.timers.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
  }

  private clearTimers(): void {
    this.clearRetryTimer();
    this.clearReconnectTimer();
  }

  /** While reconnecting, look at the daemon now and then, in case the stream will not come back. */
  private armReconnectTimer(): void {
    this.clearReconnectTimer();
    this.reconnectTimer = this.timers.setTimeout(() => {
      this.reconnectTimer = null;
      if (this.state === "reconnecting") void this.check();
    }, RECONNECT_CHECK_MS);
  }

  private notifyReconnected(): void {
    for (const listener of [...this.reconnected]) listener();
  }

  /** Asks the daemon if it is there (health, which needs no token) and if the token works (whoami). */
  private async ask(signal: AbortSignal): Promise<Outcome> {
    const call = { signal, timeoutMs: CHECK_TIMEOUT_MS };
    try {
      await this.deps.api.health(call);
      // After health, so that a dev daemon that has just started has made its token file.
      await this.deps.tokens.load();
      await this.deps.api.whoami(call);
      return { kind: "ok" };
    } catch (thrown) {
      const error = thrown instanceof ApiError ? thrown : clientError("unreachable");
      return { kind: error.code === "unauthorized" ? "unauthorized" : "unreachable", error };
    }
  }

  private async check(): Promise<void> {
    const id = ++this.checkId;
    this.abort?.abort();
    const abort = new AbortController();
    this.abort = abort;
    this.checking = true;
    this.clearRetryTimer();
    if (this.retryAt !== null)
      this.commit({ state: this.state, lastError: this.lastError, retryAt: null });
    const outcome = await this.ask(abort.signal);
    if (id !== this.checkId) return;
    this.checking = false;
    this.settle(outcome);
  }

  private settle(outcome: Outcome): void {
    if (outcome.kind === "ok") {
      this.failures = 0;
      if (this.state === "reconnecting") this.armReconnectTimer();
      else this.goOnline();
      return;
    }
    this.deps.stream.stop();
    this.clearReconnectTimer();
    if (outcome.kind === "unauthorized") {
      this.commit({ state: "unauthorized", lastError: outcome.error, retryAt: null });
      return;
    }
    const wait = backoffDelay(this.failures, CHECK_RETRY);
    this.failures += 1;
    this.commit({ state: "unreachable", lastError: outcome.error, retryAt: this.now() + wait });
    this.retryTimer = this.timers.setTimeout(() => {
      this.retryTimer = null;
      void this.check();
    }, wait);
  }

  private goOnline(): void {
    if (this.state === "online") return;
    const first = !this.everOnline;
    this.everOnline = true;
    this.commit({ state: "online", lastError: null, retryAt: null });
    this.deps.stream.start();
    if (!first) this.notifyReconnected();
  }
}

/** The plain state machine of the connection. `createConnection` wraps it in signals. */
export function createConnectionMachine(deps: MachineDeps): ConnectionMachine {
  const machine = new Machine(deps);
  // Plain functions, so a caller can spread or destructure the result without losing `this`.
  return {
    start: () => machine.start(),
    stop: () => machine.stop(),
    retryNow: () => machine.retryNow(),
    streamState: (state, detail) => machine.streamState(state, detail),
    reportUnauthorized: (error) => machine.reportUnauthorized(error),
    onReconnected: (listener) => machine.onReconnected(listener),
  };
}
