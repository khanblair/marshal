import type { SocketLike, WebSocketCtor } from "../event-stream";

/** The states of a real `WebSocket`, by the numbers the browser uses. */
const OPEN = 1;
const CLOSED = 3;
const ABNORMAL_CLOSE = 1006;
const NORMAL_CLOSE = 1000;
const FIRST_APP_CLOSE = 3000;
const LAST_APP_CLOSE = 4999;

/** A WebSocket that a test drives by hand. It records what was offered, sent, and closed. */
export class FakeWebSocket implements SocketLike {
  protocol = "";
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readonly sent: string[] = [];
  closedWith: { code: number | undefined } | null = null;

  constructor(
    readonly url: string,
    readonly offered: string[],
  ) {}

  send(data: string): void {
    this.sent.push(data);
  }

  /** Like the browser: only 1000 and 3000 to 4999 may be given, and anything else throws. */
  close(code?: number): void {
    if (
      code !== undefined &&
      code !== NORMAL_CLOSE &&
      !(code >= FIRST_APP_CLOSE && code <= LAST_APP_CLOSE)
    ) {
      throw new DOMException(
        "The close code must be 1000 or between 3000 and 4999.",
        "InvalidAccessError",
      );
    }
    this.readyState = CLOSED;
    this.closedWith = { code };
  }

  /** The daemon accepts the connection and picks a subprotocol. */
  accept(protocol = "marshal.v1"): void {
    this.protocol = protocol;
    this.readyState = OPEN;
    this.onopen?.(new Event("open"));
  }

  /** The daemon sends a frame. An object is sent as JSON, and a string as it is. */
  push(frame: unknown): void {
    const data = typeof frame === "string" ? frame : JSON.stringify(frame);
    this.onmessage?.(new MessageEvent("message", { data }));
  }

  /** The connection is lost, as when the daemon stops or the network goes. */
  drop(code = ABNORMAL_CLOSE): void {
    this.readyState = CLOSED;
    this.onclose?.(new CloseEvent("close", { code }));
  }

  /** The socket reports an error. */
  fail(): void {
    this.onerror?.(new Event("error"));
  }

  /** The Hello messages the client sent, as objects. */
  hellos(): Record<string, unknown>[] {
    return this.sent.map((text) => JSON.parse(text) as Record<string, unknown>);
  }
}

export interface FakeSockets {
  /** Hand this to the stream as `WebSocketImpl`. */
  Impl: WebSocketCtor;
  /** Every socket the stream opened, in order. */
  all: FakeWebSocket[];
  /** The newest socket. It throws when none was opened, so a test fails at the right line. */
  last(): FakeWebSocket;
}

export interface FakeSocketsOptions {
  /** Runs for every message a client sends (a `hello`, or one of a card's terminal messages), so a
   * fake daemon in memory can answer it the way the real one does. Not called for a message a test
   * pushes onto the socket itself. */
  onSend?: (socket: FakeWebSocket, data: string) => void;
}

/** A fresh set of sockets for one test, so no state is shared between tests. */
export function fakeSockets(options: FakeSocketsOptions = {}): FakeSockets {
  const all: FakeWebSocket[] = [];
  class Made extends FakeWebSocket {
    constructor(url: string, protocols?: string | string[]) {
      super(url, protocols === undefined ? [] : [protocols].flat());
      all.push(this);
    }
    override send(data: string): void {
      super.send(data);
      options.onSend?.(this, data);
    }
  }
  return {
    Impl: Made,
    all,
    last() {
      const socket = all.at(-1);
      if (!socket) throw new Error("no socket was opened");
      return socket;
    },
  };
}
