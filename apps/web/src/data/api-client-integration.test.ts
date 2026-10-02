import type { IntegrationState } from "@marshal/protocol";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { card, make } from "./api-client-test-helpers";
import { emptyAnswer, jsonAnswer } from "./testing/fake-fetch";
import { golden } from "./testing/golden";

const state = golden<IntegrationState>("integration-state");

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("the merge flow's routes", () => {
  const routes = {
    "GET /v1/projects/web/integration": () => jsonAnswer(state),
    "POST /v1/projects/web/integration/pause": () => jsonAnswer({ ...state, state: "paused" }),
    "POST /v1/projects/web/integration/resume": () => jsonAnswer(state),
    [`POST /v1/cards/${card.id}/merge/retry`]: () => jsonAnswer(card),
    [`POST /v1/cards/${card.id}/merge/undo`]: () => jsonAnswer(card),
    [`POST /v1/cards/${card.id}/worktree/open`]: () => emptyAnswer(),
  };

  it("answers the state, the card, or nothing, on the daemon's own routes", async () => {
    const { client, calls } = make(routes);
    expect(await client.integrationState("web")).toEqual(state);
    expect(await client.pauseIntegration("web")).toMatchObject({ state: "paused" });
    expect(await client.resumeIntegration("web")).toEqual(state);
    expect(await client.retryCardMerge(card.id)).toEqual(card);
    expect(await client.undoCardMerge(card.id)).toEqual(card);
    expect(await client.openCardWorktree(card.id, { with: "finder" })).toBeUndefined();
    expect(calls.map((c) => `${c.method} ${c.url}`)).toEqual(Object.keys(routes));
  });

  it("sends the way to open a folder as JSON, and no body for the rest", async () => {
    const { client, calls } = make(routes);
    await client.openCardWorktree(card.id, { with: "editor" });
    await client.retryCardMerge(card.id);
    await client.pauseIntegration("web");
    expect(JSON.parse(calls[0]?.body ?? "")).toEqual({ with: "editor" });
    expect(calls[0]?.headers["content-type"]).toBe("application/json");
    expect(calls[1]?.body).toBeNull();
    expect(calls[2]?.body).toBeNull();
  });

  it("escapes the ids it puts in a path", async () => {
    const { client, calls } = make({
      "GET /v1/projects/a%20b/integration": () => jsonAnswer(state),
    });
    await client.integrationState("a b");
    expect(calls[0]?.url).toBe("/v1/projects/a%20b/integration");
  });
});
