import type { ErrorResponse } from "@marshal/protocol";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { card, chat, failureOf, make, project, TOKEN } from "./api-client-test-helpers";
import { createDaemonClock } from "./daemon-clock";
import { emptyAnswer, errorAnswer, hang, jsonAnswer, textAnswer } from "./testing/fake-fetch";
import { golden } from "./testing/golden";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("requests", () => {
  it("sends the token, asks for JSON, and keeps cookies and caches out", async () => {
    const { client, calls } = make({ "GET /v1/projects/web-dashboard": () => jsonAnswer(project) });
    await client.getProject("web-dashboard");
    expect(calls).toHaveLength(1);
    const [call] = calls;
    expect(call?.url).toBe("/v1/projects/web-dashboard");
    expect(call?.method).toBe("GET");
    expect(call?.headers.authorization).toBe(`Bearer ${TOKEN}`);
    expect(call?.headers.accept).toBe("application/json");
    expect(call?.headers["content-type"]).toBeUndefined();
    expect(call?.body).toBeNull();
    expect(call?.credentials).toBe("omit");
    expect(call?.cache).toBe("no-store");
  });

  it("still sends a request when there is no token, and sends no Authorization header", async () => {
    const { client, calls } = make(
      { "GET /v1/health": () => jsonAnswer(golden("health")) },
      { getToken: () => null },
    );
    await client.health();
    expect(calls[0]?.headers.authorization).toBeUndefined();
  });

  it("puts the base address in front and drops a trailing slash", async () => {
    const { client, calls } = make(
      { "GET /v1/health": () => jsonAnswer(golden("health")) },
      { baseUrl: "http://127.0.0.1:47801/" },
    );
    await client.health();
    expect(calls[0]?.url).toBe("http://127.0.0.1:47801/v1/health");
  });

  it("sends a body as JSON with its content type", async () => {
    const { client, calls } = make({ "POST /v1/projects": () => jsonAnswer(project, 201) });
    const body = golden<Parameters<typeof client.createProject>[0]>("create-project-request");
    await client.createProject(body);
    expect(calls[0]?.headers["content-type"]).toBe("application/json");
    expect(JSON.parse(calls[0]?.body ?? "")).toEqual(body);
  });

  it("escapes every path segment", async () => {
    const { client, calls } = make({});
    await failureOf(client.getProject("a/b c#1"));
    expect(calls[0]?.url).toBe("/v1/projects/a%2Fb%20c%231");
  });
});

describe("failures", () => {
  it("throws the daemon's own error, with its message, code, status, and details", async () => {
    const { error } = golden<ErrorResponse>("error");
    const { client } = make({ "GET /v1/cards/nope": () => jsonAnswer({ error }, 404) });
    const failed = await failureOf(client.getCard("nope"));
    expect(failed.code).toBe("not_found");
    expect(failed.status).toBe(404);
    expect(failed.message).toBe(error.message);
    expect(failed.details).toEqual(error.details);
  });

  it("calls a network failure unreachable", async () => {
    const { client } = make({
      "GET /v1/health": () => {
        throw new TypeError("Failed to fetch");
      },
    });
    const failed = await failureOf(client.health());
    expect(failed.code).toBe("unreachable");
    expect(failed.retryable).toBe(true);
  });

  it("calls a 2xx that is not JSON a bad response", async () => {
    const { client } = make({ "GET /v1/health": () => textAnswer("<html>hi</html>", 200) });
    const failed = await failureOf(client.health());
    expect(failed.code).toBe("bad_response");
    expect(failed.status).toBeNull();
  });

  it("calls an empty 2xx a bad response where JSON is expected", async () => {
    const { client } = make({ "GET /v1/health": () => emptyAnswer(200) });
    expect((await failureOf(client.health())).code).toBe("bad_response");
  });

  it("calls a failure with no error shape a bad response and keeps its status", async () => {
    const { client } = make({
      "GET /v1/health": () => textAnswer("<html>Oops</html>", 500),
      "GET /v1/agents": () => jsonAnswer({ oops: true }, 500),
      "GET /v1/projects": () => jsonAnswer({ error: { code: "teapot", message: "x" } }, 418),
    });
    expect(await failureOf(client.health())).toMatchObject({ code: "bad_response", status: 500 });
    expect(await failureOf(client.agents())).toMatchObject({ code: "bad_response", status: 500 });
    expect(await failureOf(client.listProjects())).toMatchObject({
      code: "bad_response",
      status: 418,
    });
  });

  it.each([502, 503, 504])(
    "calls a %d that a proxy sends, with no error shape, unreachable, as when the dev daemon is down",
    async (status) => {
      const { client } = make({ "GET /v1/health": () => textAnswer("Bad Gateway", status) });
      const failed = await failureOf(client.health());
      expect(failed).toMatchObject({ code: "unreachable", status, retryable: true });
      expect(failed.message).toBe("Marshal can't reach the daemon. Check that it is running.");
    },
  );

  it("still reads the daemon's own 503 as unavailable", async () => {
    const { client } = make({
      "GET /v1/health": () => errorAnswer(503, "unavailable", "Try again in a moment."),
    });
    expect(await failureOf(client.health())).toMatchObject({ code: "unavailable", status: 503 });
  });

  it("does not touch the clock for an error answer", async () => {
    const observe = vi.fn();
    const { client } = make(
      { "GET /v1/health": () => errorAnswer(503, "unavailable", "Try again in a moment.") },
      { clock: { observe } },
    );
    await failureOf(client.health());
    expect(observe).not.toHaveBeenCalled();
  });
});

describe("401", () => {
  it("calls onUnauthorized once and throws the daemon's code", async () => {
    const onUnauthorized = vi.fn();
    const { client } = make(
      {
        "GET /v1/projects": () => errorAnswer(401, "unauthorized", "Sign in again to use Marshal."),
      },
      { onUnauthorized },
    );
    const failed = await failureOf(client.listProjects());
    expect(failed.code).toBe("unauthorized");
    expect(failed.message).toBe("Sign in again to use Marshal.");
    expect(onUnauthorized).toHaveBeenCalledTimes(1);
    expect(onUnauthorized).toHaveBeenCalledWith(failed);
    await failureOf(client.listProjects());
    expect(onUnauthorized).toHaveBeenCalledTimes(2);
  });

  it("calls it for a 401 whose body is not the error shape too", async () => {
    const onUnauthorized = vi.fn();
    const { client } = make(
      { "GET /v1/projects": () => textAnswer("no", 401) },
      { onUnauthorized },
    );
    expect((await failureOf(client.listProjects())).code).toBe("bad_response");
    expect(onUnauthorized).toHaveBeenCalledTimes(1);
  });

  it("does not call it for other failures", async () => {
    const onUnauthorized = vi.fn();
    const { client } = make(
      { "GET /v1/projects": () => errorAnswer(403, "forbidden", "That is not allowed.") },
      { onUnauthorized },
    );
    await failureOf(client.listProjects());
    expect(onUnauthorized).not.toHaveBeenCalled();
  });

  it("keeps the daemon's error when onUnauthorized itself throws", async () => {
    const { client } = make(
      { "GET /v1/projects": () => errorAnswer(401, "unauthorized", "Sign in again.") },
      {
        onUnauthorized: () => {
          throw new RangeError("a handler bug");
        },
      },
    );
    await expect(client.listProjects()).rejects.toThrow("a handler bug");
  });
});

describe("time limits and cancelling", () => {
  it("times out after the limit of the client", async () => {
    const { client } = make({ "GET /v1/health": hang }, { timeoutMs: 5000 });
    const pending = failureOf(client.health());
    await vi.advanceTimersByTimeAsync(4999);
    await vi.advanceTimersByTimeAsync(1);
    const failed = await pending;
    expect(failed.code).toBe("timeout");
    expect(failed.retryable).toBe(true);
  });

  it("uses 30 seconds by default", async () => {
    const { client } = make({ "GET /v1/health": hang });
    const pending = failureOf(client.health());
    await vi.advanceTimersByTimeAsync(29_999);
    let settled = false;
    void pending.then(() => {
      settled = true;
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(settled).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect((await pending).code).toBe("timeout");
  });

  it.each(["startCard", "resumeCard"] as const)("gives %s three minutes", async (method) => {
    const { client } = make({
      [`POST /v1/cards/${card.id}/start`]: hang,
      [`POST /v1/cards/${card.id}/resume`]: hang,
    });
    const pending = failureOf(client[method](card.id));
    let settled = false;
    void pending.then(() => {
      settled = true;
    });
    await vi.advanceTimersByTimeAsync(179_999);
    expect(settled).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect((await pending).code).toBe("timeout");
  });

  it("gives a card's view switch three minutes, since the agent's process restarts", async () => {
    const { client } = make({ [`POST /v1/cards/${card.id}/view`]: hang });
    const pending = failureOf(client.setCardView(card.id, { mode: "terminal" }));
    let settled = false;
    void pending.then(() => {
      settled = true;
    });
    await vi.advanceTimersByTimeAsync(179_999);
    expect(settled).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect((await pending).code).toBe("timeout");
  });

  it("gives a chat's message three minutes, since the first one starts its agent", async () => {
    const { client } = make({ [`POST /v1/chats/${chat.id}/messages`]: hang });
    const pending = failureOf(client.sendChatMessage(chat.id, { text: "Hello" }));
    let settled = false;
    void pending.then(() => {
      settled = true;
    });
    await vi.advanceTimersByTimeAsync(179_999);
    expect(settled).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect((await pending).code).toBe("timeout");
  });

  it("lets a chat's message call replace the three minutes", async () => {
    const { client } = make({ [`POST /v1/chats/${chat.id}/messages`]: hang });
    const pending = failureOf(
      client.sendChatMessage(chat.id, { text: "Hello" }, { timeoutMs: 100 }),
    );
    await vi.advanceTimersByTimeAsync(100);
    expect((await pending).code).toBe("timeout");
  });

  it("lets a call replace the limit", async () => {
    const { client } = make({ "GET /v1/health": hang });
    const pending = failureOf(client.health({ timeoutMs: 100 }));
    await vi.advanceTimersByTimeAsync(100);
    expect((await pending).code).toBe("timeout");
  });

  it("cancels with the caller's signal and says aborted", async () => {
    const { client } = make({ "GET /v1/health": hang });
    const controller = new AbortController();
    const pending = failureOf(client.health({ signal: controller.signal }));
    controller.abort();
    const failed = await pending;
    expect(failed.code).toBe("aborted");
    expect(failed.retryable).toBe(false);
  });

  it("says aborted for a signal that was already cancelled", async () => {
    const { client } = make({ "GET /v1/health": () => jsonAnswer(golden("health")) });
    const controller = new AbortController();
    controller.abort();
    expect((await failureOf(client.health({ signal: controller.signal }))).code).toBe("aborted");
  });

  it("leaves no timer behind after an answer", async () => {
    const { client } = make({ "GET /v1/health": () => jsonAnswer(golden("health")) });
    await client.health();
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("the daemon's clock", () => {
  it("gives the clock the server time with the send and receive times", async () => {
    const observe = vi.fn();
    vi.setSystemTime(new Date("2026-09-25T10:00:00.000Z"));
    const { client } = make(
      {
        "GET /v1/health": () => {
          vi.setSystemTime(new Date("2026-09-25T10:00:00.200Z"));
          return jsonAnswer(golden("health"));
        },
      },
      { clock: { observe } },
    );
    await client.health();
    expect(observe).toHaveBeenCalledWith({
      serverTime: (golden("health") as { serverTime: string }).serverTime,
      sentAt: Date.parse("2026-09-25T10:00:00.000Z"),
      receivedAt: Date.parse("2026-09-25T10:00:00.200Z"),
    });
  });

  it("learns an offset from an answer through a real clock", async () => {
    vi.setSystemTime(new Date("2026-09-25T10:00:00.000Z"));
    const clock = createDaemonClock();
    const { client } = make(
      {
        "GET /v1/agents": () => jsonAnswer({ agents: [], serverTime: "2026-09-25T10:00:05.000Z" }),
      },
      { clock },
    );
    await client.agents();
    expect(clock.offsetMs()).toBe(5000);
  });

  it("does not observe an answer without a server time", async () => {
    const observe = vi.fn();
    const { client } = make(
      { "GET /v1/other": () => jsonAnswer({ a: 1 }) },
      { clock: { observe } },
    );
    await client.request("GET", "/v1/other");
    expect(observe).not.toHaveBeenCalled();
  });
});
