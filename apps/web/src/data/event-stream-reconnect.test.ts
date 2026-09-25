import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EPOCH_A, errorFrame, events, setup } from "./testing/stream-harness";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("topics", () => {
  it("sends a new hello when a topic is added or removed while open", () => {
    const { stream, open } = setup();
    const socket = open();
    socket.push(events(EPOCH_A, 41));
    stream.subscribe(["home"]);
    stream.subscribe(["card:abc", "project:web-dashboard"]);
    stream.unsubscribe(["home"]);
    expect(socket.hellos().slice(1)).toEqual([
      { type: "hello", subscribe: ["home"], sinceSeq: 41, epoch: EPOCH_A },
      {
        type: "hello",
        subscribe: ["home", "card:abc", "project:web-dashboard"],
        sinceSeq: 41,
        epoch: EPOCH_A,
      },
      {
        type: "hello",
        subscribe: ["card:abc", "project:web-dashboard"],
        sinceSeq: 41,
        epoch: EPOCH_A,
      },
    ]);
  });

  it("does not send a hello for a change that changes nothing", () => {
    const { stream, open } = setup();
    const socket = open();
    stream.subscribe(["home"]);
    const before = socket.sent.length;
    stream.subscribe(["home"]);
    stream.unsubscribe(["card:never-followed"]);
    stream.subscribe([]);
    expect(socket.sent).toHaveLength(before);
  });

  it("puts topics chosen before the connection is open in the first hello", () => {
    const { stream, sockets } = setup();
    stream.start();
    stream.subscribe(["home"]);
    expect(sockets.last().sent).toEqual([]);
    sockets.last().accept();
    expect(sockets.last().hellos()[0]?.subscribe).toEqual(["home"]);
  });

  it("remembers the topics across a reconnect", () => {
    const { stream, open, sockets } = setup();
    stream.subscribe(["home"]);
    const first = open();
    stream.subscribe(["card:abc"]);
    first.drop();
    vi.advanceTimersByTime(500);
    sockets.last().accept();
    expect(sockets.last().hellos()[0]?.subscribe).toEqual(["home", "card:abc"]);
  });

  it("sends nothing while it is waiting between tries", () => {
    const { stream, open, sockets } = setup();
    const socket = open();
    socket.drop();
    const before = socket.sent.length;
    stream.subscribe(["home"]);
    expect(sockets.all).toHaveLength(1);
    expect(socket.sent).toHaveLength(before);
  });
});

describe("reconnecting", () => {
  const waits = (
    stream: ReturnType<typeof setup>["stream"],
    sockets: ReturnType<typeof setup>["sockets"],
    n: number,
  ) => {
    const found: number[] = [];
    for (let i = 0; i < n; i++) {
      sockets.last().drop();
      let waited = 0;
      while (stream.state() === "waiting") {
        vi.advanceTimersByTime(100);
        waited += 100;
      }
      found.push(waited);
    }
    return found;
  };

  it("waits 500 ms, then doubles, up to 10 seconds", () => {
    const { stream, sockets } = setup();
    stream.start();
    expect(waits(stream, sockets, 8)).toEqual([
      500, 1000, 2000, 4000, 8000, 10_000, 10_000, 10_000,
    ]);
  });

  it("moves each wait by the jitter, up to 20 percent either way", () => {
    for (const [random, expected] of [
      [0, 400],
      [1, 600],
    ] as const) {
      const { stream, sockets } = setup({ random: () => random });
      stream.start();
      sockets.last().drop();
      vi.advanceTimersByTime(expected - 1);
      expect(sockets.all).toHaveLength(1);
      vi.advanceTimersByTime(1);
      expect(sockets.all).toHaveLength(2);
    }
  });

  it("goes to waiting and says how many tries failed, and back to connecting on the retry", () => {
    const { stream, sockets, states } = setup();
    stream.start();
    sockets.last().drop();
    vi.advanceTimersByTime(500);
    sockets.last().drop();
    expect(states.map(([s, d]) => `${s}:${d.attempts}`)).toEqual([
      "connecting:0",
      "waiting:1",
      "connecting:1",
      "waiting:2",
    ]);
  });

  it("treats an error like a close, and does not retry twice for both", () => {
    const { stream, sockets } = setup();
    stream.start();
    const socket = sockets.last();
    socket.fail();
    socket.drop();
    expect(stream.state()).toBe("waiting");
    expect(vi.getTimerCount()).toBe(1);
    vi.advanceTimersByTime(500);
    expect(sockets.all).toHaveLength(2);
  });

  it("starts again from 500 ms after a first frame, an events frame, or ten quiet seconds", () => {
    const { stream, sockets, open } = setup();
    open();
    expect(waits(stream, sockets, 2)).toEqual([500, 1000]);
    sockets.last().accept();
    sockets.last().push(events(EPOCH_A, 41));
    expect(waits(stream, sockets, 1)).toEqual([500]);
    sockets.last().accept();
    vi.advanceTimersByTime(9999);
    expect(waits(stream, sockets, 1)).toEqual([1000]);
    sockets.last().accept();
    vi.advanceTimersByTime(10_000);
    expect(waits(stream, sockets, 1)).toEqual([500]);
  });

  it("does not start again from 500 ms after an error frame", () => {
    const { stream, sockets, states } = setup();
    stream.start();
    sockets.last().drop();
    vi.advanceTimersByTime(500);
    sockets.last().accept();
    sockets.last().push(errorFrame);
    expect(states.at(-1)?.[1].attempts).toBe(1);
    sockets.last().drop(1008);
    let waited = 0;
    while (stream.state() === "waiting") {
      vi.advanceTimersByTime(100);
      waited += 100;
    }
    expect(waited).toBe(1000);
  });
});

describe("stop", () => {
  it("closes the connection and leaves no timer running", () => {
    const { stream, sockets, open } = setup();
    const socket = open();
    stream.stop();
    expect(socket.closedWith).toEqual({ code: 1000 });
    expect(stream.state()).toBe("stopped");
    expect(vi.getTimerCount()).toBe(0);
    socket.drop();
    vi.advanceTimersByTime(60_000);
    expect(sockets.all).toHaveLength(1);
  });

  it("clears a retry that is waiting", () => {
    const { stream, sockets } = setup();
    stream.start();
    sockets.last().drop();
    expect(vi.getTimerCount()).toBe(1);
    stream.stop();
    expect(vi.getTimerCount()).toBe(0);
    vi.advanceTimersByTime(60_000);
    expect(sockets.all).toHaveLength(1);
  });

  it("can be called before start and twice", () => {
    const { stream, states } = setup();
    stream.stop();
    stream.stop();
    expect(states).toEqual([]);
  });

  it("starts again with the topics and the position it had", () => {
    const { stream, sockets, open } = setup();
    stream.subscribe(["home"]);
    open().push(events(EPOCH_A, 41));
    stream.stop();
    stream.start();
    sockets.last().accept();
    expect(sockets.last().hellos()).toEqual([
      { type: "hello", subscribe: ["home"], sinceSeq: 41, epoch: EPOCH_A },
    ]);
  });

  it("ignores what a socket sends after stop", () => {
    const { stream, open, batches } = setup();
    const socket = open();
    stream.stop();
    socket.push(events(EPOCH_A, 41));
    expect(batches).toEqual([]);
  });
});
