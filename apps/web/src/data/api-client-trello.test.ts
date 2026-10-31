import type { ApplyTrelloResult, RefreshTrelloResult, TrelloPending } from "@marshal/protocol";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { make } from "./api-client-test-helpers";
import { jsonAnswer } from "./testing/fake-fetch";
import { golden } from "./testing/golden";

const pending = golden<TrelloPending>("trello-pending");
const applied = golden<ApplyTrelloResult>("trello-apply-result");
const refreshed = golden<RefreshTrelloResult>("trello-refresh-result");

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("the Trello apply routes", () => {
  const routes = {
    "GET /v1/projects/web/trello/pending": () => jsonAnswer(pending),
    "POST /v1/projects/web/trello/apply": () => jsonAnswer(applied),
    "POST /v1/projects/web/trello/dismiss": () => jsonAnswer(pending),
    "POST /v1/projects/web/trello/refresh": () => jsonAnswer(refreshed),
  };

  it("answers the pending changes, the apply, the dismiss, and the refresh, on the daemon's routes", async () => {
    const { client, calls } = make(routes);
    expect(await client.trelloPending("web")).toEqual(pending);
    expect(await client.applyTrello("web", { changeIds: ["a"], overrideIds: [] })).toEqual(applied);
    expect(await client.dismissTrello("web", { changeIds: ["a"] })).toEqual(pending);
    expect(await client.refreshTrello("web", { keepTrelloCardIds: [] })).toEqual(refreshed);
    expect(calls.map((c) => `${c.method} ${c.url}`)).toEqual(Object.keys(routes));
  });

  it("sends the chosen ids as JSON, and no body for the read", async () => {
    const { client, calls } = make(routes);
    await client.trelloPending("web");
    await client.applyTrello("web", { changeIds: ["a", "b"], overrideIds: ["b"] });
    await client.dismissTrello("web", { changeIds: ["c"] });
    await client.refreshTrello("web", { keepTrelloCardIds: ["card-1"] });
    expect(calls[0]?.body).toBeNull();
    expect(JSON.parse(calls[1]?.body ?? "")).toEqual({ changeIds: ["a", "b"], overrideIds: ["b"] });
    expect(JSON.parse(calls[2]?.body ?? "")).toEqual({ changeIds: ["c"] });
    expect(JSON.parse(calls[3]?.body ?? "")).toEqual({ keepTrelloCardIds: ["card-1"] });
    expect(calls[1]?.headers["content-type"]).toBe("application/json");
  });

  it("escapes the project id it puts in a path", async () => {
    const { client, calls } = make({
      "GET /v1/projects/a%20b/trello/pending": () => jsonAnswer(pending),
    });
    await client.trelloPending("a b");
    expect(calls[0]?.url).toBe("/v1/projects/a%20b/trello/pending");
  });
});
