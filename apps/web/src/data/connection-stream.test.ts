import type { Health } from "@marshal/protocol";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { clientError } from "./api-error";
import { createConnection } from "./connection";
import {
  dropAndReturn,
  flush,
  HEALTH,
  online,
  setup,
  UNAUTHORIZED,
} from "./testing/connection-harness";
import { createTokenStore } from "./token";

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-09-25T10:00:00.000Z"));
});
afterEach(() => vi.useRealTimers());

describe("the event stream", () => {
  it("is reconnecting when the stream drops, and online again when it opens, telling the owner each time", async () => {
    const t = setup();
    await online(t);
    await dropAndReturn(t);
    expect(t.connection.state()).toBe("online");
    expect(t.reconnected).toHaveBeenCalledTimes(1);
    await dropAndReturn(t);
    expect(t.reconnected).toHaveBeenCalledTimes(2);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("does not react to a stream that is only connecting or open while online", async () => {
    const t = setup();
    await online(t);
    t.connection.streamState("connecting", { attempts: 0, error: null });
    t.connection.streamState("open", { attempts: 0, error: null });
    expect(t.connection.state()).toBe("online");
    expect(t.reconnected).not.toHaveBeenCalled();
  });

  it("checks the daemon after 15 seconds of not being open", async () => {
    const t = setup();
    await online(t);
    t.connection.streamState("waiting", { attempts: 1, error: null });
    await vi.advanceTimersByTimeAsync(14_999);
    expect(t.api.health).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(t.api.health).toHaveBeenCalledTimes(2);
  });

  it("checks the daemon after 3 failed tries, and only then", async () => {
    const t = setup();
    await online(t);
    for (const attempts of [1, 2]) {
      t.connection.streamState("waiting", { attempts, error: null });
      await flush();
    }
    expect(t.api.health).toHaveBeenCalledTimes(1);
    t.connection.streamState("waiting", { attempts: 3, error: null });
    await flush();
    expect(t.api.health).toHaveBeenCalledTimes(2);
    t.connection.streamState("waiting", { attempts: 4, error: null });
    await flush();
    expect(t.api.health).toHaveBeenCalledTimes(2);
  });

  it("becomes unreachable when the check finds no daemon, stops the stream, and retries the check", async () => {
    const t = setup();
    await online(t);
    t.connection.streamState("waiting", { attempts: 1, error: null });
    t.api.health.mockRejectedValue(clientError("unreachable"));
    await vi.advanceTimersByTimeAsync(15_000);
    expect(t.connection.state()).toBe("unreachable");
    expect(t.stream.stop).toHaveBeenCalled();
    t.api.health.mockResolvedValue(HEALTH);
    await vi.advanceTimersByTimeAsync(1000);
    expect(t.connection.state()).toBe("online");
    expect(t.stream.start).toHaveBeenCalledTimes(2);
    expect(t.reconnected).toHaveBeenCalledTimes(1);
  });

  it("becomes unauthorized when the check finds the token refused", async () => {
    const t = setup();
    await online(t);
    t.connection.streamState("waiting", { attempts: 1, error: null });
    t.api.whoami.mockRejectedValueOnce(UNAUTHORIZED());
    await vi.advanceTimersByTimeAsync(15_000);
    expect(t.connection.state()).toBe("unauthorized");
    expect(vi.getTimerCount()).toBe(0);
  });

  it("stays reconnecting when the daemon answers but the stream is not back, and looks again later", async () => {
    const t = setup();
    await online(t);
    t.connection.streamState("waiting", { attempts: 1, error: null });
    await vi.advanceTimersByTimeAsync(15_000);
    expect(t.connection.state()).toBe("reconnecting");
    expect(t.reconnected).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(15_000);
    expect(t.api.health).toHaveBeenCalledTimes(3);
    t.connection.streamState("open", { attempts: 2, error: null });
    expect(t.connection.state()).toBe("online");
    expect(vi.getTimerCount()).toBe(0);
  });

  it("ignores the stream before it is started and after stop", async () => {
    const t = setup();
    t.connection.streamState("waiting", { attempts: 1, error: null });
    expect(t.connection.state()).toBe("starting");
    await online(t);
    t.connection.stop();
    t.connection.streamState("waiting", { attempts: 1, error: null });
    expect(t.connection.state()).toBe("online");
  });
});

describe("the page", () => {
  it("checks at once when the page becomes visible again", async () => {
    const t = setup();
    await online(t);
    Object.assign(t.page.document, { visibilityState: "hidden" });
    t.page.document.dispatchEvent(new Event("visibilitychange"));
    await flush();
    expect(t.api.health).toHaveBeenCalledTimes(1);
    Object.assign(t.page.document, { visibilityState: "visible" });
    t.page.document.dispatchEvent(new Event("visibilitychange"));
    await flush();
    expect(t.api.health).toHaveBeenCalledTimes(2);
  });

  it("checks at once when the browser says it is online", async () => {
    const t = setup();
    t.api.health.mockRejectedValue(clientError("unreachable"));
    t.connection.start();
    await flush();
    t.api.health.mockResolvedValue(HEALTH);
    t.page.window.dispatchEvent(new Event("online"));
    await flush();
    expect(t.connection.state()).toBe("online");
  });

  it("does not check after stop", async () => {
    const t = setup();
    await online(t);
    t.connection.stop();
    t.page.window.dispatchEvent(new Event("online"));
    t.page.document.dispatchEvent(new Event("visibilitychange"));
    await flush();
    expect(t.api.health).toHaveBeenCalledTimes(1);
  });

  it("works without page signals", async () => {
    const t = setup();
    const tokens = createTokenStore({ storage: null });
    const bare = createConnection({ api: t.api, tokens, stream: t.stream });
    bare.start();
    await flush();
    expect(bare.state()).toBe("online");
  });
});

describe("stop and overlapping checks", () => {
  it("leaves no timer running and stops the stream", async () => {
    const t = setup();
    t.api.health.mockRejectedValue(clientError("unreachable"));
    t.connection.start();
    await flush();
    expect(vi.getTimerCount()).toBe(1);
    t.connection.stop();
    expect(vi.getTimerCount()).toBe(0);
    expect(t.stream.stop).toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(t.api.health).toHaveBeenCalledTimes(1);
  });

  it("clears the reconnect timer too", async () => {
    const t = setup();
    await online(t);
    t.connection.streamState("waiting", { attempts: 1, error: null });
    expect(vi.getTimerCount()).toBe(1);
    t.connection.stop();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("ignores a check that finishes after stop", async () => {
    const t = setup();
    let release: () => void = () => undefined;
    t.api.health.mockImplementationOnce(
      () => new Promise<Health>((resolve) => (release = () => resolve(HEALTH))),
    );
    t.connection.start();
    t.connection.stop();
    release();
    await flush();
    expect(t.connection.state()).toBe("starting");
    expect(t.stream.start).not.toHaveBeenCalled();
  });

  it("cancels the check that is running when a new one starts, and ignores its answer", async () => {
    const t = setup();
    let release: (error: Error) => void = () => undefined;
    let firstSignal: AbortSignal | undefined;
    t.api.health.mockImplementationOnce((options) => {
      firstSignal = options?.signal;
      return new Promise<Health>((_resolve, reject) => (release = reject));
    });
    t.connection.start();
    t.connection.retryNow();
    await flush();
    expect(firstSignal?.aborted).toBe(true);
    expect(t.connection.state()).toBe("online");
    release(clientError("aborted"));
    await flush();
    expect(t.connection.state()).toBe("online");
  });

  it("can be started again after stop", async () => {
    const t = setup();
    await online(t);
    t.connection.stop();
    t.connection.start();
    await flush();
    expect(t.api.health).toHaveBeenCalledTimes(2);
  });

  it("stops telling a listener that unsubscribed", async () => {
    const t = setup();
    const listener = vi.fn();
    const off = t.connection.onReconnected(listener);
    await online(t);
    off();
    await dropAndReturn(t);
    expect(listener).not.toHaveBeenCalled();
    expect(t.reconnected).toHaveBeenCalledTimes(1);
  });
});
