// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createData } from "./index";
import { fakeFetch, jsonAnswer } from "./testing/fake-fetch";
import { fakeSockets } from "./testing/fake-web-socket";
import { golden } from "./testing/golden";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("createData with no window", () => {
  it("has no browser globals, and still builds and runs from what it is given", async () => {
    expect(typeof window).toBe("undefined");
    expect(typeof document).toBe("undefined");
    expect(typeof location).toBe("undefined");
    const http = fakeFetch({
      "GET /v1/health": () => jsonAnswer(golden("health")),
      "GET /v1/auth/whoami": () => jsonAnswer(golden("whoami")),
    });
    const sockets = fakeSockets();
    const data = createData({
      storage: null,
      fetch: http.fetch,
      WebSocketImpl: sockets.Impl,
      page: { protocol: "http:", host: "localhost:3210" },
    });
    data.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(data.connection.state()).toBe("online");
    data.stop();
    expect(vi.getTimerCount()).toBe(0);
  });
});
