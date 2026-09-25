import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toEventsUrl } from "./stream-frames";
import { fakeSockets } from "./testing/fake-web-socket";
import {
  batchFrame,
  EPOCH_A,
  errorFrame,
  events,
  helloFrame,
  resyncFrame,
  STREAM_URL,
  setup,
  TOKEN,
} from "./testing/stream-harness";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("connecting", () => {
  it("offers marshal.v1 and the token as a subprotocol, and puts no token in the address", () => {
    const { stream, sockets } = setup();
    stream.start();
    const socket = sockets.last();
    expect(socket.url).toBe(STREAM_URL);
    expect(socket.url).not.toContain(TOKEN);
    expect(socket.offered).toEqual(["marshal.v1", `bearer.${TOKEN}`]);
    expect(stream.state()).toBe("connecting");
  });

  it("offers only marshal.v1 when there is no token", () => {
    const { stream, sockets } = setup({ getToken: () => null });
    stream.start();
    expect(sockets.last().offered).toEqual(["marshal.v1"]);
  });

  it("reads the token again on each try", () => {
    let token = "first";
    const { stream, sockets } = setup({ getToken: () => token });
    stream.start();
    token = "second";
    sockets.last().drop();
    vi.advanceTimersByTime(500);
    expect(sockets.last().offered).toEqual(["marshal.v1", "bearer.second"]);
  });

  it("sends a hello first, with the topics, and it is open after that", () => {
    const { stream, sockets, states } = setup();
    stream.subscribe(["home", "project:web-dashboard"]);
    stream.start();
    const socket = sockets.last();
    expect(socket.sent).toEqual([]);
    socket.accept();
    expect(socket.hellos()).toEqual([
      { type: "hello", subscribe: ["home", "project:web-dashboard"], sinceSeq: 0, epoch: "" },
    ]);
    expect(stream.state()).toBe("open");
    expect(states.map(([state]) => state)).toEqual(["connecting", "open"]);
  });

  it("sends the same hello the Go tests wrote, with the stored epoch and last seq", () => {
    const { stream, open, sockets } = setup();
    stream.subscribe(helloFrame.subscribe);
    open();
    sockets.last().push(events(EPOCH_A, 41));
    sockets.last().drop();
    vi.advanceTimersByTime(500);
    sockets.last().accept();
    expect(sockets.last().hellos()).toEqual([helloFrame]);
  });

  it("closes a connection that did not pick marshal.v1 and tries again", () => {
    const { stream, sockets } = setup();
    stream.start();
    const socket = sockets.last();
    socket.accept("something.else");
    expect(socket.closedWith).toEqual({ code: 1000 });
    expect(socket.sent).toEqual([]);
    expect(stream.state()).toBe("waiting");
    vi.advanceTimersByTime(500);
    expect(sockets.all).toHaveLength(2);
  });

  it("tries again when the WebSocket cannot even be made", () => {
    let made = 0;
    const sockets = fakeSockets();
    const Impl = class extends sockets.Impl {
      constructor(url: string, protocols?: string | string[]) {
        if (made++ === 0) throw new SyntaxError("bad subprotocol");
        super(url, protocols);
      }
    };
    const { stream } = setup({ WebSocketImpl: Impl });
    stream.start();
    expect(stream.state()).toBe("waiting");
    vi.advanceTimersByTime(500);
    expect(stream.state()).toBe("connecting");
  });

  it("does not open a second connection when started twice", () => {
    const { stream, sockets } = setup();
    stream.start();
    stream.start();
    expect(sockets.all).toHaveLength(1);
  });

  it("uses the page's WebSocket when none is given", () => {
    const sockets = fakeSockets();
    const original = globalThis.WebSocket;
    globalThis.WebSocket = sockets.Impl as unknown as typeof WebSocket;
    try {
      const { stream } = setup({ WebSocketImpl: undefined });
      stream.start();
      expect(sockets.all).toHaveLength(1);
    } finally {
      globalThis.WebSocket = original;
    }
  });
});

describe("frames", () => {
  it("hands on new events in order with their epoch", () => {
    const { open, batches, resyncs } = setup();
    const socket = open();
    expect(resyncs).toEqual([{ ...resyncFrame, epoch: EPOCH_A, seq: 40 }]);
    socket.push(batchFrame);
    expect(batches).toEqual([{ seqs: [41, 42], epoch: EPOCH_A }]);
  });

  it("ignores events that were already applied", () => {
    const { open, batches } = setup();
    const socket = open();
    socket.push(events(EPOCH_A, 41, 42));
    socket.push(events(EPOCH_A, 42, 43));
    socket.push(events(EPOCH_A, 41, 42, 43));
    expect(batches.map((b) => b.seqs)).toEqual([[41, 42], [43]]);
  });

  it("does not take a gap in the numbers for a loss", () => {
    const { open, batches, resyncs } = setup();
    const socket = open();
    socket.push(events(EPOCH_A, 41));
    socket.push(events(EPOCH_A, 45, 90));
    expect(batches.map((b) => b.seqs)).toEqual([[41], [45, 90]]);
    expect(resyncs).toHaveLength(1);
  });

  it("keeps events in the order they came and drops a number that goes backwards", () => {
    const { open, batches } = setup();
    open().push(events(EPOCH_A, 50, 49, 51));
    expect(batches[0]?.seqs).toEqual([50, 51]);
  });

  it("treats a different epoch on an events frame as a resync and applies its events", () => {
    const { open, batches, resyncs } = setup();
    const socket = open();
    socket.push(events(EPOCH_A, 41, 42));
    socket.push(events("01M3C0ZZZZ000000000000000B", 1, 2));
    expect(resyncs.at(-1)).toEqual({
      type: "resync",
      epoch: "01M3C0ZZZZ000000000000000B",
      reason: "epoch-changed",
      seq: 0,
    });
    // The new numbers start again from 1, so the old position must not hide them.
    expect(batches.at(-1)).toEqual({ seqs: [1, 2], epoch: "01M3C0ZZZZ000000000000000B" });
  });

  it("stores the epoch and seq of a resync frame and says hello with them next time", () => {
    const { stream, open, sockets, resyncs } = setup();
    const socket = open();
    socket.push({ ...resyncFrame, epoch: "EPOCH-NEXT", seq: 7 });
    expect(resyncs.at(-1)?.seq).toBe(7);
    // Events after the resync seq follow on the same connection.
    socket.push(events("EPOCH-NEXT", 7, 8));
    socket.drop();
    vi.advanceTimersByTime(500);
    sockets.last().accept();
    expect(sockets.last().hellos()[0]).toMatchObject({ epoch: "EPOCH-NEXT", sinceSeq: 8 });
    expect(stream.state()).toBe("open");
  });

  it("passes on an error frame through the state, and the daemon's close ends the connection", () => {
    const { open, states, stream, sockets } = setup();
    const socket = open();
    socket.push(errorFrame);
    const [state, detail] = states.at(-1) ?? ["stopped", { attempts: 0, error: null }];
    expect(state).toBe("open");
    expect(detail.error).toEqual(errorFrame.error);
    socket.drop(1008);
    expect(stream.state()).toBe("waiting");
    expect(sockets.all).toHaveLength(1);
  });

  it("ignores and counts a message that is not JSON, has an unknown type, or is not text", () => {
    const { open, stream, batches } = setup();
    const socket = open();
    socket.push("not json {");
    socket.push({ type: "mystery" });
    socket.push({ type: "events", epoch: 4 });
    socket.push({ type: "resync", epoch: EPOCH_A, seq: 1, reason: "made-up" });
    socket.push({ type: "error", error: { code: "nope" } });
    socket.push("42");
    socket.onmessage?.(new MessageEvent("message", { data: new ArrayBuffer(2) }));
    expect(stream.ignoredFrames()).toBe(7);
    expect(stream.state()).toBe("open");
    socket.push(events(EPOCH_A, 41));
    expect(batches).toHaveLength(1);
  });

  it("leaves out an event it cannot read and uses the rest of the frame", () => {
    const { open, stream, batches } = setup();
    const socket = open();
    const frame = events(EPOCH_A, 41, 42, 43);
    socket.push({
      ...frame,
      events: [
        frame.events[0],
        { ...frame.events[1], type: "not.a.type.yet" },
        { seq: "x" },
        frame.events[2],
      ],
    });
    expect(batches[0]?.seqs).toEqual([41, 43]);
    expect(stream.ignoredFrames()).toBe(2);
  });
});

describe("toEventsUrl", () => {
  it.each([
    ["", { protocol: "http:", host: "localhost:3210" }, "ws://localhost:3210/v1/events"],
    ["", { protocol: "https:", host: "marshal.example" }, "wss://marshal.example/v1/events"],
    [
      "http://127.0.0.1:47800",
      { protocol: "https:", host: "ignored" },
      "ws://127.0.0.1:47800/v1/events",
    ],
    [
      "https://box.tail.net",
      { protocol: "http:", host: "ignored" },
      "wss://box.tail.net/v1/events",
    ],
  ])("turns %j on %j into %s", (base, page, expected) => {
    expect(toEventsUrl(base, page)).toBe(expected);
  });
});
