import type { Resync, Event as WireEvent } from "@marshal/protocol";
import { type ApiClient, createApiClient } from "./api-client";
import { type Connection, createConnection } from "./connection";
import { createDaemonClock, type DaemonClock } from "./daemon-clock";
import { createDevTokenFetcher } from "./dev-token-client";
import { createEventStream, type EventStream, type WebSocketCtor } from "./event-stream";
import { watchPage } from "./page-signals";
import type { KeyValueStore } from "./storage";
import { type Frame, toEventsUrl } from "./stream-frames";
import type { Timers } from "./timers";
import { createTokenStore, type TokenStore } from "./token";

export interface DataOptions {
  /** The daemon's address. Empty means the page's own address, which the dev server forwards to the dev daemon. */
  baseUrl?: string;
  /** Where the token is kept: `localStorage`, or null when the page has none. */
  storage: KeyValueStore | null;
  fetch?: typeof fetch;
  WebSocketImpl?: WebSocketCtor;
  /**
   * True in the dev build (`import.meta.env.DEV`). Only then does the token store ask the dev
   * server for the dev daemon's token, unless `fetchDevToken` says how.
   */
  dev?: boolean;
  /** Replaces the way the dev token is read from the dev server. A test uses it. */
  fetchDevToken?: () => Promise<string | null>;
  /** The page's own address, for the event stream when there is no `baseUrl`. It is `location` by default. */
  page?: { protocol: string; host: string };
  /** Signals that the page came back to the front or the network returned. The page's own by default. */
  watchPage?: (onReturn: () => void) => () => void;
  timers?: Timers;
  /** Events the app has not seen yet, from the event stream. */
  onBatch?: (events: WireEvent[], epoch: string) => void;
  /** Events were missed, so reload the snapshots. */
  onResync?: (frame: Resync) => void;
  /** A card's terminal channel answered or refused (docs/architecture.md 11.2). */
  onTerminalFrame?: (frame: Frame, cardId: string) => void;
}

/** Everything the app needs to talk to the daemon, built once and passed around. */
export interface Data {
  tokens: TokenStore;
  clock: DaemonClock;
  api: ApiClient;
  stream: EventStream;
  connection: Connection;
  /** Runs the function for each batch of events the stream delivers, in order. Returns a way to stop. */
  onEvents(listener: (events: WireEvent[], epoch: string) => void): () => void;
  /** Runs the function each time the stream says events were missed, so the snapshots must be loaded again. */
  onResync(listener: (frame: Resync) => void): () => void;
  /** Runs the function for each `terminal.screen` or `terminal.refused` frame a card's channel gets. */
  onTerminalFrame(listener: (frame: Frame, cardId: string) => void): () => void;
  /** Starts the connection: the first check, then the event stream. */
  start(): void;
  /** Stops the connection, the event stream, and every timer. */
  stop(): void;
}

/**
 * How the dev token is read, when the app runs in dev. The literal `import.meta.env.DEV` is
 * replaced by `false` in a production build, so the bundler can drop this call and, with it, the
 * address of the dev route. (Expected, and not proven until something imports this module.)
 */
function devTokenFetcher(options: DataOptions): (() => Promise<string | null>) | undefined {
  return import.meta.env.DEV && options.dev ? createDevTokenFetcher(options.fetch) : undefined;
}

/** The page's own signals, when there is a page. In a test or a worker there is none. */
function pageSignals(): DataOptions["watchPage"] {
  if (typeof window === "undefined" || typeof document === "undefined") return undefined;
  return watchPage({ window, document });
}

/**
 * Puts the parts together: the token store, the daemon's clock, the API client, the event
 * stream, and the connection. Everything comes in through the options, so a test builds one with
 * fakes. Nothing here touches `window` until it is called, and there is no module-level state.
 */
export function createData(options: DataOptions): Data {
  const { baseUrl = "", timers } = options;
  const tokens = createTokenStore({
    storage: options.storage,
    dev: options.dev ?? false,
    fetchDevToken: options.fetchDevToken ?? devTokenFetcher(options),
  });
  const clock = createDaemonClock();
  const eventListeners = new Set<(events: WireEvent[], epoch: string) => void>();
  const resyncListeners = new Set<(frame: Resync) => void>();
  const terminalListeners = new Set<(frame: Frame, cardId: string) => void>();
  // The client, the stream, and the connection report to each other, so the connection is made last.
  const link: { connection: Connection | null } = { connection: null };
  const api = createApiClient({
    baseUrl,
    getToken: tokens.get,
    clock,
    fetch: options.fetch,
    onUnauthorized: (error) => link.connection?.reportUnauthorized(error),
  });
  const stream = createEventStream({
    url: toEventsUrl(baseUrl, options.page ?? globalThis.location),
    getToken: tokens.get,
    WebSocketImpl: options.WebSocketImpl,
    timers,
    onBatch: (events, epoch) => {
      options.onBatch?.(events, epoch);
      for (const listener of [...eventListeners]) listener(events, epoch);
    },
    onResync: (frame) => {
      options.onResync?.(frame);
      for (const listener of [...resyncListeners]) listener(frame);
    },
    onState: (state, detail) => link.connection?.streamState(state, detail),
    onTerminalFrame: (frame, cardId) => {
      options.onTerminalFrame?.(frame, cardId);
      for (const listener of [...terminalListeners]) listener(frame, cardId);
    },
  });
  const connection = createConnection({
    api,
    tokens,
    stream,
    timers,
    watchPage: options.watchPage ?? pageSignals(),
  });
  link.connection = connection;
  return {
    tokens,
    clock,
    api,
    stream,
    connection,
    onEvents: (listener) => {
      eventListeners.add(listener);
      return () => {
        eventListeners.delete(listener);
      };
    },
    onResync: (listener) => {
      resyncListeners.add(listener);
      return () => {
        resyncListeners.delete(listener);
      };
    },
    onTerminalFrame: (listener) => {
      terminalListeners.add(listener);
      return () => {
        terminalListeners.delete(listener);
      };
    },
    start: connection.start,
    stop: connection.stop,
  };
}
