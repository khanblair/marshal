import type { EventBatch, Resync } from "@marshal/protocol";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createDevTokenFetcher } from "./dev-token-client";
import { createData } from "./index";
import { emptyAnswer, errorAnswer, fakeFetch, jsonAnswer } from "./testing/fake-fetch";
import { fakeSockets } from "./testing/fake-web-socket";
import { golden } from "./testing/golden";
import { memoryStorage } from "./testing/memory-storage";
import { TOKEN_KEY } from "./token";

const TOKEN = "stored-token-not-real";
const DEV = "dev-token-not-real";
const PAGE = { protocol: "http:", host: "localhost:3210" };
const flush = () => vi.advanceTimersByTimeAsync(0);

function setup(
  extra: Partial<Parameters<typeof createData>[0]> = {},
  routes: Parameters<typeof fakeFetch>[0] = {},
) {
  const http = fakeFetch({
    "GET /v1/health": () => jsonAnswer(golden("health")),
    "GET /v1/auth/whoami": () => jsonAnswer(golden("whoami")),
    ...routes,
  });
  const sockets = fakeSockets();
  const batches: number[][] = [];
  const resyncs: Resync[] = [];
  const data = createData({
    storage: memoryStorage({ [TOKEN_KEY]: TOKEN }),
    fetch: http.fetch,
    WebSocketImpl: sockets.Impl,
    page: PAGE,
    watchPage: undefined,
    onBatch: (events) => batches.push(events.map((e) => e.seq)),
    onResync: (frame) => resyncs.push(frame),
    ...extra,
  });
  return { data, http, sockets, batches, resyncs };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("createData", () => {
  it("checks the daemon with the stored token, then opens the event stream with it", async () => {
    const { data, http, sockets } = setup();
    expect(data.connection.state()).toBe("starting");
    data.start();
    await flush();
    expect(data.connection.state()).toBe("online");
    expect(http.calls.map((c) => c.url)).toEqual(["/v1/health", "/v1/auth/whoami"]);
    expect(http.calls[1]?.headers.authorization).toBe(`Bearer ${TOKEN}`);
    const socket = sockets.last();
    expect(socket.url).toBe("ws://localhost:3210/v1/events");
    expect(socket.offered).toEqual(["marshal.v1", `bearer.${TOKEN}`]);
  });

  it("sends the base address to the client and turns it into the stream's address", async () => {
    const { data, http, sockets } = setup({ baseUrl: "https://box.tail.net" });
    data.start();
    await flush();
    expect(http.calls[0]?.url).toBe("https://box.tail.net/v1/health");
    expect(sockets.last().url).toBe("wss://box.tail.net/v1/events");
  });

  it("hands events and resyncs to its owner", async () => {
    const { data, sockets, batches, resyncs } = setup();
    data.start();
    await flush();
    data.stream.subscribe(["home"]);
    sockets.last().accept();
    const resync = golden<Resync>("resync");
    sockets.last().push(resync);
    sockets.last().push({ ...golden<EventBatch>("event-batch"), epoch: resync.epoch });
    expect(resyncs).toHaveLength(1);
    expect(batches).toEqual([[41, 42]]);
  });

  it("lets more than one listener hear events and resyncs, and lets each stop listening", async () => {
    const { data, sockets, batches } = setup();
    const heard: number[][] = [];
    const resynced = vi.fn();
    const stopEvents = data.onEvents((events) => heard.push(events.map((e) => e.seq)));
    const stopResync = data.onResync(resynced);
    data.start();
    await flush();
    sockets.last().accept();
    const resync = golden<Resync>("resync");
    sockets.last().push(resync);
    const batch = { ...golden<EventBatch>("event-batch"), epoch: resync.epoch };
    sockets.last().push(batch);
    expect(heard).toEqual([[41, 42]]);
    expect(batches).toEqual([[41, 42]]);
    expect(resynced).toHaveBeenCalledOnce();
    stopEvents();
    stopResync();
    sockets.last().push({ ...batch, events: batch.events.map((e) => ({ ...e, seq: e.seq + 10 })) });
    sockets.last().push({ ...resync, epoch: "01M3C0ZZZZ000000000000000C" });
    expect(heard).toHaveLength(1);
    expect(resynced).toHaveBeenCalledOnce();
  });

  it("hands a card's terminal.screen and terminal.refused frames to its owner and every listener", async () => {
    const { data, sockets } = setup();
    const heard: [string, string][] = [];
    const stop = data.onTerminalFrame((frame, cardId) => heard.push([frame.kind, cardId]));
    data.start();
    await flush();
    sockets.last().accept();
    const cardId = "01M3C107JB041061050R3GG28A";
    sockets
      .last()
      .push({ type: "terminal.screen", cardId, cols: 120, rows: 32, throughSeq: 0, data: "" });
    sockets.last().push({
      type: "terminal.refused",
      cardId,
      error: { code: "refused", message: "nope", details: { reason: "terminal_busy" } },
    });
    expect(heard).toEqual([
      ["terminal.screen", cardId],
      ["terminal.refused", cardId],
    ]);
    stop();
    sockets
      .last()
      .push({ type: "terminal.screen", cardId, cols: 120, rows: 32, throughSeq: 1, data: "" });
    expect(heard).toHaveLength(2);
  });

  it("learns the daemon's clock from the answers", async () => {
    const { data } = setup();
    vi.setSystemTime(new Date("2026-09-25T10:15:30.123Z"));
    data.start();
    await flush();
    // The golden health answer has the server time 10:15:30.123, and no time passed in the test.
    expect(data.clock.offsetMs()).toBe(0);
    expect(Math.abs(data.clock.now() - Date.now())).toBeLessThan(5);
  });

  it("goes unauthorized when a call gets a 401, and stops the stream", async () => {
    const { data, sockets } = setup(
      {},
      {
        "GET /v1/projects": () => errorAnswer(401, "unauthorized", "Sign in again to use Marshal."),
      },
    );
    data.start();
    await flush();
    sockets.last().accept();
    await expect(data.api.listProjects()).rejects.toMatchObject({ code: "unauthorized" });
    expect(data.connection.state()).toBe("unauthorized");
    expect(sockets.last().closedWith).toEqual({ code: 1000 });
    // The daemon still accepts the token on the look with a fresh one, so the app is back.
    await flush();
    expect(data.connection.state()).toBe("online");
  });

  it("recovers by itself when the stream drops and returns, and tells the owner", async () => {
    const { data, sockets } = setup();
    const back = vi.fn();
    data.connection.onReconnected(back);
    data.start();
    await flush();
    sockets.last().accept();
    sockets.last().drop();
    expect(data.connection.state()).toBe("reconnecting");
    vi.advanceTimersByTime(1000);
    sockets.last().accept();
    expect(data.connection.state()).toBe("online");
    expect(back).toHaveBeenCalledTimes(1);
  });

  it("asks the dev server for the dev token when dev is true, and keeps it out of storage", async () => {
    const storage = memoryStorage();
    const { data, http } = setup(
      { storage, dev: true },
      { "GET /__marshal/dev-token": () => jsonAnswer({ token: DEV }) },
    );
    data.start();
    await flush();
    expect(data.connection.state()).toBe("online");
    const [, whoami] = http.calls.filter((c) => c.url.startsWith("/v1/"));
    expect(whoami?.headers.authorization).toBe(`Bearer ${DEV}`);
    expect(http.calls.filter((c) => c.url === "/__marshal/dev-token")).toHaveLength(1);
    expect(storage.writes).toEqual([]);
  });

  it("never asks the dev server when dev is false", async () => {
    const { data, http } = setup(
      { storage: memoryStorage(), dev: false },
      {
        "GET /__marshal/dev-token": () => jsonAnswer({ token: DEV }),
        "GET /v1/auth/whoami": () => errorAnswer(401, "unauthorized", "Sign in to use Marshal."),
      },
    );
    data.start();
    await flush();
    expect(http.calls.some((c) => c.url === "/__marshal/dev-token")).toBe(false);
    expect(data.connection.state()).toBe("unauthorized");
    expect(data.tokens.get()).toBeNull();
  });

  it("lets a caller replace how the dev token is read", async () => {
    const dev = fakeFetch({
      "GET /__marshal/dev-token": () => jsonAnswer({ token: "other-dev-token" }),
    });
    const { data, http } = setup({
      storage: memoryStorage(),
      dev: true,
      fetchDevToken: createDevTokenFetcher(dev.fetch),
    });
    data.start();
    await flush();
    expect(data.connection.state()).toBe("online");
    expect(http.calls.some((c) => c.url === "/__marshal/dev-token")).toBe(false);
    expect(dev.calls).toHaveLength(1);
    expect(data.tokens.get()).toBe("other-dev-token");
  });

  it("shows unreachable when the daemon does not answer, and comes back when it does", async () => {
    let up = false;
    const { data } = setup(
      {},
      {
        "GET /v1/health": () => {
          if (!up) throw new TypeError("Failed to fetch");
          return jsonAnswer(golden("health"));
        },
      },
    );
    data.start();
    await flush();
    expect(data.connection.state()).toBe("unreachable");
    expect(data.connection.lastError()?.message).toBe(
      "Marshal can't reach the daemon. Check that it is running.",
    );
    up = true;
    await vi.advanceTimersByTimeAsync(1000);
    expect(data.connection.state()).toBe("online");
  });

  it("stops everything and leaves no timer behind", async () => {
    const { data, sockets } = setup();
    data.start();
    await flush();
    sockets.last().accept();
    data.stop();
    expect(vi.getTimerCount()).toBe(0);
    expect(sockets.last().closedWith).toEqual({ code: 1000 });
    expect(data.stream.state()).toBe("stopped");
  });

  it("watches the page when there is one and no signals were given", async () => {
    const { data, http } = setup({ watchPage: undefined });
    data.start();
    await flush();
    const before = http.calls.length;
    window.dispatchEvent(new Event("online"));
    await flush();
    expect(http.calls.length).toBeGreaterThan(before);
    data.stop();
  });

  it("uses the page's location for the stream when there is no base address or page given", async () => {
    const sockets = fakeSockets();
    const { data } = setup({ page: undefined, WebSocketImpl: sockets.Impl });
    data.start();
    await flush();
    expect(sockets.last().url).toBe(`ws://${window.location.host}/v1/events`);
  });

  it("gives an empty answer for a 204 like a stop", async () => {
    const { data } = setup({}, { "POST /v1/cards/abc/stop": () => emptyAnswer() });
    await expect(data.api.stopCard("abc")).resolves.toBeUndefined();
  });
});
