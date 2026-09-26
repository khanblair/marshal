import {
  BearerSubprotocolPrefix,
  type ClientFrame,
  FrameTypeResync,
  type Resync,
  ResyncReasonEpochChanged,
  type Topic,
  WebSocketSubprotocol,
  type Error as WireError,
  type Event as WireEvent,
} from "@marshal/protocol";
import { type BackoffPolicy, backoffDelay } from "./backoff";
import { buildHello, type Frame, parseFrame } from "./stream-frames";
import { StreamPosition } from "./stream-position";
import { realTimers, type TimerId, type Timers } from "./timers";

/** `stopped` before `start` and after `stop`; `waiting` is the pause between two tries. */
export type StreamState = "stopped" | "connecting" | "open" | "waiting";

export interface StreamDetail {
  /** How many tries in a row have failed. It goes back to 0 once the stream has proved itself. */
  attempts: number;
  /** The last error frame the daemon sent, or null. */
  error: WireError | null;
}

/** What the stream needs from a WebSocket. The browser's own class fits, and so does a fake. */
export interface SocketLike {
  readonly protocol: string;
  onopen: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  onerror: ((event: Event) => void) | null;
  send(data: string): void;
  close(code?: number, reason?: string): void;
}

export type WebSocketCtor = new (url: string, protocols?: string | string[]) => SocketLike;

export interface EventStreamOptions {
  /** The address of the stream, with no token in it. */
  url: string;
  getToken: () => string | null;
  WebSocketImpl?: WebSocketCtor;
  timers?: Timers;
  /** A number from 0 up to 1, for the jitter of the wait between tries. */
  random?: () => number;
  /** Events the app has not seen yet, in order, with the epoch they belong to. */
  onBatch: (events: WireEvent[], epoch: string) => void;
  /** The daemon (or a new epoch) says that events were missed: reload the snapshots. */
  onResync: (frame: Resync) => void;
  onState: (state: StreamState, detail: StreamDetail) => void;
  /**
   * A card's terminal channel answered or refused (docs/architecture.md 11.2): a `terminal.screen`
   * or a `terminal.refused` frame. `cardId` is read out of whichever one it is, so the owner never
   * has to switch on `frame.kind` just to find it.
   */
  onTerminalFrame: (frame: Frame, cardId: string) => void;
}

export interface EventStream {
  start(): void;
  /** Closes the connection and clears every timer. The topics and the position are kept. */
  stop(): void;
  subscribe(topics: readonly Topic[]): void;
  unsubscribe(topics: readonly Topic[]): void;
  state(): StreamState;
  /** How many messages, or events in them, were left out because they could not be read. */
  ignoredFrames(): number;
  /**
   * Sends a card's terminal message (`terminal.input`, `terminal.resize`, or `terminal.snapshot`)
   * over the current socket. A silent no-op while the socket is not open: there is nothing to
   * resend later, since the daemon neither queues nor replays a terminal message either.
   */
  sendTerminal(frame: ClientFrame): void;
}

/** 500 ms, then 1 s, 2 s, and on up to 10 s, each moved by up to 20 percent either way. */
const RETRY: BackoffPolicy = { baseMs: 500, capMs: 10_000, jitter: 0.2 };
/** A connection that lasts this long is counted as a good one, so the next drop starts again at 500 ms. */
const STABLE_MS = 10_000;
const NORMAL_CLOSE = 1000;

class Stream implements EventStream {
  private readonly topics = new Set<Topic>();
  private readonly position = new StreamPosition();
  private readonly timers: Timers;
  private readonly random: () => number;
  private running = false;
  private current: StreamState = "stopped";
  private socket: SocketLike | null = null;
  private attempts = 0;
  private ignored = 0;
  private lastError: WireError | null = null;
  private retryTimer: TimerId | null = null;
  private stableTimer: TimerId | null = null;

  constructor(private readonly options: EventStreamOptions) {
    this.timers = options.timers ?? realTimers;
    this.random = options.random ?? Math.random;
  }

  start(): void {
    if (this.running) return;
    this.running = true;
    this.attempts = 0;
    this.connect();
  }

  stop(): void {
    this.running = false;
    this.clear(this.retryTimer);
    this.retryTimer = null;
    this.release(NORMAL_CLOSE);
    if (this.current !== "stopped") this.setState("stopped");
  }

  subscribe(topics: readonly Topic[]): void {
    const added = topics.filter((topic) => !this.topics.has(topic));
    for (const topic of added) this.topics.add(topic);
    if (added.length > 0) this.sendHello();
  }

  unsubscribe(topics: readonly Topic[]): void {
    const removed = topics.filter((topic) => this.topics.delete(topic));
    if (removed.length > 0) this.sendHello();
  }

  state(): StreamState {
    return this.current;
  }

  ignoredFrames(): number {
    return this.ignored;
  }

  sendTerminal(frame: ClientFrame): void {
    if (this.current !== "open") return;
    this.socket?.send(JSON.stringify(frame));
  }

  private setState(next: StreamState): void {
    this.current = next;
    this.options.onState(next, { attempts: this.attempts, error: this.lastError });
  }

  private clear(id: TimerId | null): void {
    if (id !== null) this.timers.clearTimeout(id);
  }

  /** Sends the topics with the position. The daemon replaces its list, and replays what was missed. */
  private sendHello(): void {
    if (this.current !== "open") return;
    this.socket?.send(
      JSON.stringify(buildHello(this.topics, this.position.seq, this.position.epoch)),
    );
  }

  private connect(): void {
    this.retryTimer = null;
    const token = this.options.getToken();
    const offered = token
      ? [WebSocketSubprotocol, BearerSubprotocolPrefix + token]
      : [WebSocketSubprotocol];
    this.setState("connecting");
    const Socket: WebSocketCtor = this.options.WebSocketImpl ?? globalThis.WebSocket;
    let ws: SocketLike;
    try {
      ws = new Socket(this.options.url, offered);
    } catch {
      this.drop();
      return;
    }
    this.socket = ws;
    ws.onopen = () => this.opened(ws);
    ws.onmessage = (event) => this.handleMessage(parseFrame(event.data));
    ws.onclose = () => this.drop();
    ws.onerror = () => this.drop();
  }

  private opened(ws: SocketLike): void {
    if (ws.protocol !== WebSocketSubprotocol) {
      this.drop();
      return;
    }
    this.lastError = null;
    this.setState("open");
    this.sendHello();
    this.stableTimer = this.timers.setTimeout(() => {
      this.attempts = 0;
    }, STABLE_MS);
  }

  /** Lets go of the current socket without waiting for its close event. */
  private release(code: number): void {
    this.clear(this.stableTimer);
    this.stableTimer = null;
    const old = this.socket;
    this.socket = null;
    if (!old) return;
    old.onopen = old.onmessage = old.onclose = old.onerror = null;
    old.close(code);
  }

  /** The connection ended or could not start. Try again after a wait, unless `stop` was called. */
  private drop(): void {
    this.release(NORMAL_CLOSE);
    if (!this.running) return;
    const wait = backoffDelay(this.attempts, RETRY, this.random);
    this.attempts += 1;
    this.setState("waiting");
    this.retryTimer = this.timers.setTimeout(() => this.connect(), wait);
  }

  private handleMessage(frame: Frame | null): void {
    if (frame === null) {
      this.ignored += 1;
      return;
    }
    switch (frame.kind) {
      case "events":
        this.ignored += frame.dropped;
        this.handleEvents(frame.frame.epoch, frame.frame.events);
        break;
      case "resync":
        this.attempts = 0;
        this.position.resync(frame.frame);
        this.options.onResync(frame.frame);
        break;
      case "error":
        // The daemon closes right after this, and the owner reads the error from the state.
        this.lastError = frame.error;
        this.options.onState(this.current, { attempts: this.attempts, error: this.lastError });
        break;
      case "terminal.screen":
      case "terminal.refused":
        this.options.onTerminalFrame(frame, frame.frame.cardId);
        break;
    }
  }

  private handleEvents(epoch: string, events: WireEvent[]): void {
    this.attempts = 0;
    const { restarted, fresh } = this.position.accept(epoch, events);
    // A new epoch on an events frame means the daemon started again. The events are still applied,
    // and the owner is told to reload, the same as after a resync frame.
    if (restarted) {
      this.options.onResync({
        type: FrameTypeResync,
        epoch,
        reason: ResyncReasonEpochChanged,
        seq: 0,
      });
    }
    if (fresh.length > 0) this.options.onBatch(fresh, epoch);
  }
}

/**
 * The one WebSocket to the daemon (architecture.md section 11.5). It says hello with the topics
 * and where it left off, hands on the events it has not seen, and asks its owner to reload when
 * events were missed. When the connection drops it tries again by itself. A gap in the numbers
 * is normal, because a client sees only the topics it follows, so a gap is never treated as loss.
 */
export function createEventStream(options: EventStreamOptions): EventStream {
  const stream = new Stream(options);
  // Plain functions, so a caller can spread or destructure the result without losing `this`.
  return {
    start: () => stream.start(),
    stop: () => stream.stop(),
    subscribe: (topics) => stream.subscribe(topics),
    unsubscribe: (topics) => stream.unsubscribe(topics),
    state: () => stream.state(),
    ignoredFrames: () => stream.ignoredFrames(),
    sendTerminal: (frame) => stream.sendTerminal(frame),
  };
}
