import { SearchHitsPerKind } from "@marshal/protocol";
import { afterEach, describe, expect, it } from "vitest";
import { ApiError } from "~/data/api-error";
import { cardId, wireCard } from "./fake-cards";
import { wireChat } from "./fake-chats";
import { createFakeDaemon, type FakeDaemon } from "./fake-daemon";
import { PROTOTYPE_PROJECTS } from "./projects";

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

const open = (options: Parameters<typeof createFakeDaemon>[0] = {}): FakeDaemon => {
  daemon = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, ...options });
  return daemon;
};

const rate = () =>
  wireCard({
    projectId: "api",
    number: 43,
    title: "Add rate limiting per API key",
    body: "Limit each key to 100 requests a minute.",
    state: "needs",
  });
const refresh = () =>
  wireCard({ projectId: "api", number: 41, title: "Fix token refresh on login", state: "working" });
const timeout = () =>
  wireCard({
    projectId: "web",
    id: cardId(7),
    number: 7,
    title: "Show the session timeout",
    state: "backlog",
  });

describe("the fake daemon's search route", () => {
  it("answers an empty or blank query with three empty lists and no work", async () => {
    const d = open({ cards: [rate()] });
    for (const q of ["", "   "]) {
      const answer = await d.data.api.search(q);
      expect(answer).toMatchObject({
        query: "",
        projects: [],
        cards: [],
        chats: [],
        totals: { projects: 0, cards: 0, chats: 0 },
      });
    }
  });

  it("answers the query it searched: trimmed, with each run of white space made one space", async () => {
    const d = open();
    expect((await d.data.api.search("  api    gateway ")).query).toBe("api gateway");
  });

  it("finds a project by its name, its id, or its folder, and gives the language", async () => {
    const d = open();
    for (const q of ["gateway", "API", "code/api"]) {
      const { projects } = await d.data.api.search(q);
      expect(projects.map((p) => p.projectId)).toEqual(["api"]);
    }
    const [hit] = (await d.data.api.search("gateway")).projects;
    expect(hit).toEqual({
      projectId: "api",
      name: "api-gateway",
      path: "~/code/api-gateway",
      language: "Go",
    });
  });

  it("matches every word, in any order, ignoring case, and names the card's project", async () => {
    const d = open({ cards: [rate(), refresh(), timeout()] });
    const { cards, totals } = await d.data.api.search("LIMITING rate");
    expect(cards).toEqual([
      {
        cardId: cardId(43),
        key: "api#43",
        number: 43,
        title: "Add rate limiting per API key",
        state: "needs",
        projectId: "api",
        projectName: "api-gateway",
      },
    ]);
    expect(totals.cards).toBe(1);
    expect((await d.data.api.search("rate nothing")).cards).toEqual([]);
  });

  it("searches a card's description as well as its title", async () => {
    const d = open({ cards: [rate(), refresh()] });
    const { cards } = await d.data.api.search("100 requests");
    expect(cards.map((c) => c.key)).toEqual(["api#43"]);
  });

  it("finds a card by its number: #41, api#41, and 41", async () => {
    const d = open({ cards: [rate(), refresh(), timeout()] });
    for (const q of ["#41", "api#41", "41"]) {
      const { cards } = await d.data.api.search(q);
      expect(cards[0]?.key, q).toBe("api#41");
    }
    expect((await d.data.api.search("web#41")).cards).toEqual([]);
  });

  it("ranks a title above a description", async () => {
    const d = open({
      cards: [
        wireCard({ projectId: "api", number: 1, title: "Nothing", body: "About tokens." }),
        wireCard({ projectId: "api", number: 2, title: "Tokens", body: "" }),
      ],
    });
    expect((await d.data.api.search("tokens")).cards.map((c) => c.key)).toEqual(["api#2", "api#1"]);
  });

  it("finds live chats only, and names their project", async () => {
    const d = open({
      chats: [
        wireChat({ id: "01M3CHAT00000000000000000A", projectId: "api", title: "Token rotation" }),
        wireChat({
          id: "01M3CHAT00000000000000000B",
          projectId: "web",
          title: "Token cleanup",
          archivedAt: "2026-09-30T10:00:00.000Z",
        }),
      ],
    });
    const { chats, totals } = await d.data.api.search("token");
    expect(chats).toEqual([
      {
        chatId: "01M3CHAT00000000000000000A",
        title: "Token rotation",
        projectId: "api",
        projectName: "api-gateway",
        lastActiveAt: expect.any(String),
      },
    ]);
    expect(totals.chats).toBe(1);
  });

  it("cuts each kind to the cap and says how many matched", async () => {
    const many = Array.from({ length: SearchHitsPerKind + 4 }, (_, i) =>
      wireCard({ projectId: "api", number: i + 1, title: `Card about search ${i + 1}` }),
    );
    const d = open({ cards: many });
    const { cards, totals } = await d.data.api.search("search");
    expect(cards).toHaveLength(SearchHitsPerKind);
    expect(totals.cards).toBe(SearchHitsPerKind + 4);
  });

  it("refuses a query of more than 200 characters with the daemon's own sentence", async () => {
    const d = open();
    const error = await d.data.api.search("a".repeat(201)).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).message).toBe("Search for 200 characters or fewer.");
    expect((await d.data.api.search("a".repeat(200))).query).toHaveLength(200);
  });

  it("wants the token like every other route", async () => {
    const d = open();
    d.revokeToken();
    const error = await d.data.api.search("api").catch((e: unknown) => e);
    expect((error as ApiError).status).toBe(401);
  });
});
