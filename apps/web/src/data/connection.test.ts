import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { clientError, wireError } from "./api-error";
import { flush, online, setup, UNAUTHORIZED } from "./testing/connection-harness";

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-09-25T10:00:00.000Z"));
});
afterEach(() => vi.useRealTimers());

describe("the first check", () => {
  it("is starting until the checks are done, then online, and starts the stream", async () => {
    const t = setup();
    expect(t.connection.state()).toBe("starting");
    t.connection.start();
    expect(t.connection.state()).toBe("starting");
    await flush();
    expect(t.connection.state()).toBe("online");
    expect(t.connection.lastError()).toBeNull();
    expect(t.connection.retryAt()).toBeNull();
    expect(t.stream.start).toHaveBeenCalledTimes(1);
  });

  it("asks health first, which needs no token, loads the token, and then asks whoami", async () => {
    const t = setup();
    await online(t);
    expect(t.calls).toEqual(["health", "load", "whoami"]);
  });

  it("gives the checks a short time limit and a signal", async () => {
    const t = setup();
    await online(t);
    for (const call of [t.api.health.mock.calls[0]?.[0], t.api.whoami.mock.calls[0]?.[0]]) {
      expect(call?.timeoutMs).toBe(5000);
      expect(call?.signal).toBeInstanceOf(AbortSignal);
    }
  });

  it("does not tell the owner it came back the first time it is online", async () => {
    const t = setup();
    await online(t);
    expect(t.reconnected).not.toHaveBeenCalled();
  });

  it("does nothing when started twice", async () => {
    const t = setup();
    t.connection.start();
    t.connection.start();
    await flush();
    expect(t.api.health).toHaveBeenCalledTimes(1);
  });
});

describe("unreachable", () => {
  it("waits 1, 2, 4, 8, then 10 seconds between checks, and says when the next one is", async () => {
    const t = setup();
    t.api.health.mockRejectedValue(clientError("unreachable"));
    t.connection.start();
    await flush();
    expect(t.connection.state()).toBe("unreachable");
    expect(t.connection.lastError()?.code).toBe("unreachable");
    for (const wait of [1000, 2000, 4000, 8000, 10_000, 10_000]) {
      expect(t.connection.retryAt()).toBe(Date.now() + wait);
      const before = t.api.health.mock.calls.length;
      await vi.advanceTimersByTimeAsync(wait - 1);
      expect(t.api.health).toHaveBeenCalledTimes(before);
      await vi.advanceTimersByTimeAsync(1);
      expect(t.api.health).toHaveBeenCalledTimes(before + 1);
    }
    expect(t.stream.start).not.toHaveBeenCalled();
  });

  it("recovers by itself when the daemon comes back, then goes on to whoami", async () => {
    const t = setup();
    t.api.health
      .mockRejectedValueOnce(clientError("unreachable"))
      .mockRejectedValueOnce(clientError("timeout"));
    t.connection.start();
    await flush();
    await vi.advanceTimersByTimeAsync(1000);
    expect(t.connection.state()).toBe("unreachable");
    expect(t.connection.lastError()?.code).toBe("timeout");
    await vi.advanceTimersByTimeAsync(2000);
    expect(t.connection.state()).toBe("online");
    expect(t.connection.lastError()).toBeNull();
    expect(t.connection.retryAt()).toBeNull();
    expect(t.api.whoami).toHaveBeenCalledTimes(1);
    expect(t.stream.start).toHaveBeenCalledTimes(1);
    // It is the first time online, so the owner loads for the first time by itself.
    expect(t.reconnected).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("counts an answer that is not the daemon's, such as a proxy's error page, as unreachable", async () => {
    const t = setup();
    t.api.health.mockRejectedValue(clientError("bad_response", 500));
    t.connection.start();
    await flush();
    expect(t.connection.state()).toBe("unreachable");
    expect(t.connection.lastError()?.code).toBe("bad_response");
  });

  it("counts a failed whoami that is not a 401 as unreachable too, and tries again", async () => {
    const t = setup();
    t.api.whoami.mockRejectedValueOnce(
      wireError(503, { code: "unavailable", message: "Try again in a moment." }),
    );
    t.connection.start();
    await flush();
    expect(t.connection.state()).toBe("unreachable");
    await vi.advanceTimersByTimeAsync(1000);
    expect(t.connection.state()).toBe("online");
  });

  it("counts a failure that is not an ApiError as unreachable", async () => {
    const t = setup();
    t.api.health.mockRejectedValueOnce(new TypeError("boom"));
    t.connection.start();
    await flush();
    expect(t.connection.state()).toBe("unreachable");
    expect(t.connection.lastError()?.code).toBe("unreachable");
  });

  it("retryNow checks at once and starts the waits again from a second", async () => {
    const t = setup();
    t.api.health.mockRejectedValue(clientError("unreachable"));
    t.connection.start();
    await flush();
    await vi.advanceTimersByTimeAsync(1000);
    await vi.advanceTimersByTimeAsync(2000);
    expect(t.connection.retryAt()).toBe(Date.now() + 4000);
    t.connection.retryNow();
    await flush();
    expect(t.api.health).toHaveBeenCalledTimes(4);
    expect(t.connection.retryAt()).toBe(Date.now() + 1000);
  });
});

describe("unauthorized", () => {
  it("stops retrying when whoami says 401, and waits", async () => {
    const t = setup();
    t.api.whoami.mockRejectedValue(UNAUTHORIZED());
    t.connection.start();
    await flush();
    expect(t.connection.state()).toBe("unauthorized");
    expect(t.connection.lastError()?.code).toBe("unauthorized");
    expect(t.connection.retryAt()).toBeNull();
    expect(t.stream.start).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
    await vi.advanceTimersByTimeAsync(120_000);
    expect(t.api.whoami).toHaveBeenCalledTimes(1);
  });

  it("checks again when the token changes, and is online when it works", async () => {
    const t = setup();
    t.api.whoami.mockRejectedValueOnce(UNAUTHORIZED());
    t.connection.start();
    await flush();
    t.tokens.set("a-new-token");
    await flush();
    expect(t.connection.state()).toBe("online");
    expect(t.stream.start).toHaveBeenCalledTimes(1);
  });

  it("stays unauthorized when the new token is refused too", async () => {
    const t = setup();
    t.api.whoami.mockRejectedValue(UNAUTHORIZED());
    t.connection.start();
    await flush();
    t.tokens.set("still-wrong");
    await flush();
    expect(t.connection.state()).toBe("unauthorized");
    expect(t.api.whoami).toHaveBeenCalledTimes(2);
  });

  it("checks again on retryNow", async () => {
    const t = setup();
    t.api.whoami.mockRejectedValueOnce(UNAUTHORIZED());
    t.connection.start();
    await flush();
    t.connection.retryNow();
    await flush();
    expect(t.connection.state()).toBe("online");
  });

  it("goes unauthorized when any call gets a 401, stops the stream, and looks once with a fresh token", async () => {
    const t = setup();
    await online(t);
    t.api.whoami.mockRejectedValueOnce(UNAUTHORIZED());
    const error = UNAUTHORIZED();
    t.connection.reportUnauthorized(error);
    expect(t.connection.state()).toBe("unauthorized");
    expect(t.connection.lastError()).toBe(error);
    expect(t.stream.stop).toHaveBeenCalled();
    await flush();
    // The look with a fresh token was refused too, so it stays, and it does not look again.
    expect(t.connection.state()).toBe("unauthorized");
    t.connection.reportUnauthorized(UNAUTHORIZED());
    await flush();
    expect(t.api.whoami).toHaveBeenCalledTimes(2);
    expect(t.load).toHaveBeenCalledTimes(2);
  });

  it("is online again, and says so, when the look with a fresh token works", async () => {
    const t = setup();
    await online(t);
    t.connection.reportUnauthorized(UNAUTHORIZED());
    await flush();
    expect(t.connection.state()).toBe("online");
    expect(t.stream.start).toHaveBeenCalledTimes(2);
    expect(t.reconnected).toHaveBeenCalledTimes(1);
  });

  it("ignores a 401 that a running check reads itself", async () => {
    const t = setup();
    t.api.whoami.mockImplementationOnce(async () => {
      t.connection.reportUnauthorized(UNAUTHORIZED());
      throw UNAUTHORIZED();
    });
    t.connection.start();
    await flush();
    expect(t.connection.state()).toBe("unauthorized");
    expect(t.api.health).toHaveBeenCalledTimes(1);
  });

  it("ignores a 401 report after stop", async () => {
    const t = setup();
    await online(t);
    t.connection.stop();
    t.connection.reportUnauthorized(UNAUTHORIZED());
    expect(t.connection.state()).toBe("online");
  });
});
