import { SearchHitsPerKind, type SearchSnapshot } from "@marshal/protocol";
import { createRoot } from "solid-js";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type SectionId, type SectionStatus, sectionStatus } from "~/data/sections";
import { golden } from "~/data/testing/golden";
import { tone } from "~/mock/constants";
import { cardId, wireCard } from "~/testing/fake-cards";
import { wireChat } from "~/testing/fake-chats";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS, wireProject } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { cleanQuery, createPaletteSearch, hitsOf, SEARCH_DEBOUNCE_MS } from "./search";

// Section S24a: the palette's search (docs/backend-checklist.md B2.11). It is switched, and these
// tests still say so in their own table (with the cards and the chats on the daemon too, which the
// rows they open need), so they keep testing the daemon half on the day the register changes.
const ON_DAEMON: Readonly<Record<SectionId, SectionStatus>> = {
  ...sectionStatus,
  S5a: "daemon",
  S17: "daemon",
  S24a: "daemon",
};

const CHAT_ID = "01M3CHAT00000000000000000A";

const refresh = () =>
  wireCard({ projectId: "api", number: 41, title: "Fix token refresh on login", state: "working" });
const rate = () =>
  wireCard({
    projectId: "api",
    number: 43,
    title: "Add rate limiting per API key",
    state: "needs",
  });
const timeout = () =>
  wireCard({
    projectId: "web",
    id: cardId(7),
    number: 7,
    title: "Show the session timeout",
    state: "backlog",
  });
const rotation = () =>
  wireChat({ id: CHAT_ID, projectId: "api", title: "Token rotation question" });

let daemon: FakeDaemon | null = null;
afterEach(() => {
  vi.useRealTimers();
  daemon?.data.stop();
  daemon = null;
});

/** A store that follows a daemon with the given contents, waited for until its first snapshots are in. */
async function synced(
  options: Parameters<typeof createFakeDaemon>[0] = {},
  sections: Readonly<Record<SectionId, SectionStatus>> = ON_DAEMON,
) {
  const d = createFakeDaemon({
    projects: PROTOTYPE_PROJECTS,
    cards: [refresh(), rate(), timeout()],
    chats: [rotation()],
    ...options,
  });
  daemon = d;
  const M = createTestMarshal({ data: d.data, sections });
  await d.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  return { d, M, ctx: contextOf(M) };
}

/** Lets typing pause long enough for the search to be sent, and its answer to come back. */
async function pause(): Promise<void> {
  await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
}

const searches = (d: FakeDaemon): string[] =>
  d.routes().filter((r) => r.startsWith("GET /v1/search"));

describe("cleanQuery", () => {
  it("trims, makes each run of white space one space, and keeps the case", () => {
    expect(cleanQuery("  Rate \t\n limiting  ")).toBe("Rate limiting");
    expect(cleanQuery("   ")).toBe("");
    expect(cleanQuery("api#41")).toBe("api#41");
  });
});

describe("a palette search", () => {
  it("sends nothing for a blank query, and a query only once typing has paused", async () => {
    const { d, ctx } = await synced();
    vi.useFakeTimers();
    const search = createPaletteSearch(ctx);
    search.ask("");
    search.ask("   ");
    await pause();
    expect(searches(d)).toEqual([]);
    search.ask("a");
    search.ask("ap");
    search.ask("api");
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS - 1);
    expect(searches(d)).toEqual([]);
    await vi.advanceTimersByTimeAsync(1);
    expect(searches(d)).toEqual(["GET /v1/search?q=api"]);
    await vi.waitFor(() => expect(search.hits()?.query).toBe("api"));
  });

  it("sends the query as the daemon reads it, and asks again only when it changes", async () => {
    const { d, ctx } = await synced();
    vi.useFakeTimers();
    const search = createPaletteSearch(ctx);
    search.ask("  rate    limiting ");
    await pause();
    await vi.waitFor(() => expect(search.hits()?.query).toBe("rate limiting"));
    search.ask("rate limiting  ");
    await pause();
    expect(searches(d)).toEqual(["GET /v1/search?q=rate+limiting"]);
  });

  it("makes the daemon's answer into the palette's rows, in the shape the mock's rows have", async () => {
    const { ctx } = await synced();
    vi.useFakeTimers();
    const search = createPaletteSearch(ctx);
    search.ask("token");
    await pause();
    await vi.waitFor(() => expect(search.hits()).not.toBeNull());
    const hits = search.hits();
    expect(hits?.projects).toEqual([]);
    expect(hits?.cards.map((row) => [row.group, row.label, row.card, row.icon])).toEqual([
      ["Cards", "api-gateway #41 Fix token refresh on login", "api#41", "st-working"],
    ]);
    expect(hits?.cards[0]?.iconColor).toBe(tone("working", "solid"));
    expect(hits?.chats.map((row) => [row.group, row.label, row.hint])).toEqual([
      ["Chats", "Token rotation question", "api-gateway"],
    ]);
  });

  it("gives a project row its language as the hint", async () => {
    const { ctx } = await synced();
    vi.useFakeTimers();
    const search = createPaletteSearch(ctx);
    search.ask("dashboard");
    await pause();
    await vi.waitFor(() => expect(search.hits()).not.toBeNull());
    expect(search.hits()?.projects.map((row) => [row.group, row.label, row.hint])).toEqual([
      ["Projects", "web-dashboard", "TypeScript"],
    ]);
  });

  it("drops the answer of an older query that arrives after the newest has been answered", async () => {
    const { d, ctx } = await synced();
    vi.useFakeTimers();
    const release = d.holdNext("GET /v1/search?q=ap");
    const search = createPaletteSearch(ctx);
    search.ask("ap");
    await pause();
    expect(searches(d)).toEqual(["GET /v1/search?q=ap"]);
    search.ask("api");
    await pause();
    await vi.waitFor(() => expect(search.hits()?.query).toBe("api"));
    release();
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(0);
    expect(search.hits()?.query).toBe("api");
  });

  it("drops an answer whose query is not the newest, whatever request it came from", async () => {
    const { d, ctx } = await synced();
    vi.useFakeTimers();
    const answer = golden<SearchSnapshot>("search");
    vi.spyOn(d.data.api, "search").mockResolvedValue({ ...answer, query: "something else" });
    const search = createPaletteSearch(ctx);
    search.ask("token");
    await pause();
    await vi.advanceTimersByTimeAsync(0);
    expect(d.data.api.search).toHaveBeenCalledTimes(1);
    expect(search.hits()).toBeNull();
  });

  it("has no answer for a query that was changed while it was in flight", async () => {
    const { d, ctx } = await synced();
    vi.useFakeTimers();
    const release = d.holdNext("GET /v1/search?q=token");
    const search = createPaletteSearch(ctx);
    search.ask("token");
    await pause();
    search.ask("");
    release();
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(0);
    expect(search.hits()).toBeNull();
  });

  it("lets go of the answer when the query is cleared, and sends nothing for it", async () => {
    const { d, ctx } = await synced();
    vi.useFakeTimers();
    const search = createPaletteSearch(ctx);
    search.ask("token");
    await pause();
    await vi.waitFor(() => expect(search.hits()).not.toBeNull());
    search.ask("");
    await pause();
    expect(search.hits()).toBeNull();
    expect(searches(d)).toEqual(["GET /v1/search?q=token"]);
  });

  it("has no answer when the request fails, so the palette shows what the store holds", async () => {
    const { d, ctx, M } = await synced();
    vi.useFakeTimers();
    d.refuseNext("GET /v1/search?q=token", 500, "internal", "Marshal hit a problem.");
    const search = createPaletteSearch(ctx);
    search.ask("token");
    await pause();
    await vi.advanceTimersByTimeAsync(0);
    expect(searches(d)).toEqual(["GET /v1/search?q=token"]);
    expect(search.hits()).toBeNull();
    // Nothing is said about it: the daemon's offline banner is the shell's, and a failed search
    // would otherwise toast on every pause in typing.
    expect(M.S.toasts).toEqual([]);
  });

  it("has no answer when the daemon stopped answering, and answers again once it is back", async () => {
    const { d, ctx } = await synced();
    vi.useFakeTimers();
    const search = createPaletteSearch(ctx);
    d.stop();
    search.ask("token");
    await pause();
    await vi.advanceTimersByTimeAsync(0);
    expect(search.hits()).toBeNull();
    d.start();
    search.ask("token x");
    await pause();
    await vi.waitFor(() => expect(search.hits()?.query).toBe("token x"));
  });

  it("sends nothing while the connection is not online", async () => {
    const { d, ctx } = await synced();
    vi.useFakeTimers();
    ctx.S.connection = { ...(ctx.S.connection ?? {}), state: "reconnecting" } as never;
    const search = createPaletteSearch(ctx);
    search.ask("token");
    await pause();
    expect(searches(d)).toEqual([]);
    expect(search.hits()).toBeNull();
  });

  it("sends nothing while S24a is on the mock", async () => {
    const { d, ctx } = await synced({}, { ...ON_DAEMON, S24a: "mock" });
    vi.useFakeTimers();
    const search = createPaletteSearch(ctx);
    search.ask("token");
    await pause();
    expect(searches(d)).toEqual([]);
    expect(search.hits()).toBeNull();
  });

  it("sends nothing when there is no daemon at all", async () => {
    vi.useFakeTimers();
    const ctx = contextOf(createTestMarshal({ sections: ON_DAEMON }));
    const search = createPaletteSearch(ctx);
    search.ask("token");
    await pause();
    expect(search.hits()).toBeNull();
  });

  it("does not send a query the daemon would refuse for its length, and counts characters", async () => {
    const { d, ctx } = await synced();
    vi.useFakeTimers();
    const search = createPaletteSearch(ctx);
    search.ask("é".repeat(201));
    await pause();
    expect(searches(d)).toEqual([]);
    search.ask("é".repeat(200));
    await pause();
    expect(searches(d)).toHaveLength(1);
  });

  it("cancels what is waiting when it is stopped", async () => {
    const { d, ctx } = await synced();
    vi.useFakeTimers();
    const search = createPaletteSearch(ctx);
    search.ask("token");
    search.stop();
    await pause();
    expect(searches(d)).toEqual([]);
  });

  it("stops by itself when the component that made it is disposed", async () => {
    const { d, ctx } = await synced();
    vi.useFakeTimers();
    let search = null as ReturnType<typeof createPaletteSearch> | null;
    const dispose = createRoot((dispose) => {
      search = createPaletteSearch(ctx);
      return dispose;
    });
    search?.ask("token");
    dispose();
    await pause();
    expect(searches(d)).toEqual([]);
  });
});

describe("the rows of an answer", () => {
  const answer = (over: Partial<SearchSnapshot> = {}): SearchSnapshot => ({
    ...golden<SearchSnapshot>("search"),
    ...over,
  });

  it("keeps at most the daemon's cap of each kind, even if more were sent", async () => {
    const { ctx } = await synced();
    const base = answer();
    const many = <T>(one: T | undefined): T[] =>
      Array.from({ length: SearchHitsPerKind + 5 }, () => one as T);
    const hits = hitsOf(
      ctx,
      answer({
        projects: many(base.projects[0]),
        cards: many(base.cards[0]),
        chats: many(base.chats[0]),
      }),
    );
    expect(hits.projects).toHaveLength(SearchHitsPerKind);
    expect(hits.cards).toHaveLength(SearchHitsPerKind);
    expect(hits.chats).toHaveLength(SearchHitsPerKind);
  });

  it("leaves the chats out while they are the mock's, since the daemon's ids are not theirs", async () => {
    const { ctx } = await synced({}, { ...ON_DAEMON, S17: "mock" });
    const hits = hitsOf(ctx, answer());
    expect(hits.chats).toEqual([]);
    expect(hits.cards.length).toBeGreaterThan(0);
  });

  it("carries the query the daemon answered", async () => {
    const { ctx } = await synced();
    expect(hitsOf(ctx, answer({ query: "api#41" })).query).toBe("api#41");
  });
});

describe("opening a row", () => {
  const toasts = (M: { S: { toasts: { msg: string }[] } }): string[] =>
    M.S.toasts.map((t) => t.msg);

  it("opens a project on its own page", async () => {
    const { M, ctx } = await synced();
    M.go("home");
    const hits = hitsOf(ctx, {
      ...golden<SearchSnapshot>("search"),
      projects: [{ projectId: "web", name: "web-dashboard", path: "", language: "TypeScript" }],
    });
    hits.projects[0]?.run();
    await vi.waitFor(() => expect(M.S.route).toMatchObject({ page: "project", pid: "web" }));
  });

  it("reads the store again for a project it does not hold, and says so when it is still not there", async () => {
    const { d, M, ctx } = await synced();
    M.go("home");
    const before = d.routes().filter((r) => r === "GET /v1/projects").length;
    hitsOf(ctx, golden<SearchSnapshot>("search")).projects[0]?.run();
    await vi.waitFor(() =>
      expect(toasts(M)).toEqual(["Marshal cannot find that project. It may have been removed."]),
    );
    expect(d.routes().filter((r) => r === "GET /v1/projects").length).toBeGreaterThan(before);
    expect(M.S.route.page).toBe("home");
  });

  it("opens a card the store holds in its panel, without asking the daemon for it", async () => {
    const { d, M, ctx } = await synced();
    const hits = hitsOf(ctx, {
      ...golden<SearchSnapshot>("search"),
      cards: [
        {
          cardId: cardId(41),
          key: "api#41",
          number: 41,
          title: "Fix token refresh on login",
          state: "working",
          projectId: "api",
          projectName: "api-gateway",
        },
      ],
    });
    hits.cards[0]?.run();
    await vi.waitFor(() => expect(M.S.openId).toBe("api#41"));
    expect(M.S.route.pid).toBe("api");
    expect(d.routes()).not.toContain(`GET /v1/cards/${cardId(41)}`);
  });

  it("reads a card the store does not hold yet, then opens it", async () => {
    const { d, M, ctx } = await synced();
    const fresh = wireCard({
      projectId: "api",
      number: 60,
      title: "Made elsewhere",
      state: "review",
    });
    d.cards.push(fresh);
    expect(M.card("api#60")).toBeUndefined();
    const hits = hitsOf(ctx, {
      ...golden<SearchSnapshot>("search"),
      cards: [
        {
          cardId: fresh.id,
          key: "api#60",
          number: 60,
          title: "Made elsewhere",
          state: "review",
          projectId: "api",
          projectName: "api-gateway",
        },
      ],
    });
    hits.cards[0]?.run();
    await vi.waitFor(() => expect(M.S.openId).toBe("api#60"));
    expect(d.routes()).toContain(`GET /v1/cards/${fresh.id}`);
  });

  it("says so, in the daemon's words, when the card has been removed since the search", async () => {
    const { M, ctx } = await synced();
    const hits = hitsOf(ctx, {
      ...golden<SearchSnapshot>("search"),
      cards: [
        {
          cardId: cardId(99),
          key: "api#99",
          number: 99,
          title: "Gone",
          state: "done",
          projectId: "api",
          projectName: "api-gateway",
        },
      ],
    });
    hits.cards[0]?.run();
    await vi.waitFor(() => expect(toasts(M)).toHaveLength(1));
    expect(toasts(M)[0]).toMatch(/cannot find that card/);
    expect(M.S.openId).toBeNull();
  });

  it("reads everything again for a project that has just arrived, then opens its card", async () => {
    const { d, M, ctx } = await synced();
    const added = wireProject({ id: "docs", name: "docs-site", language: "Markdown" });
    const card = wireCard({
      projectId: "docs",
      number: 3,
      title: "Write the guide",
      state: "planning",
    });
    d.projects.push(added);
    d.cards.push(card);
    expect(M.S.projects.some((p) => p.id === "docs")).toBe(false);
    const hits = hitsOf(ctx, {
      ...golden<SearchSnapshot>("search"),
      cards: [
        {
          cardId: card.id,
          key: "docs#3",
          number: 3,
          title: "Write the guide",
          state: "planning",
          projectId: "docs",
          projectName: "docs-site",
        },
      ],
    });
    hits.cards[0]?.run();
    await vi.waitFor(() => expect(M.S.openId).toBe("docs#3"));
    expect(M.S.projects.some((p) => p.id === "docs")).toBe(true);
  });

  it("opens a chat in its project's chat view, with that chat chosen", async () => {
    const { M, ctx } = await synced();
    const hits = hitsOf(ctx, {
      ...golden<SearchSnapshot>("search"),
      chats: [
        {
          chatId: CHAT_ID,
          title: "Token rotation question",
          projectId: "api",
          projectName: "api-gateway",
          lastActiveAt: "2026-09-30T10:00:00.000Z",
        },
      ],
    });
    M.go("home");
    hits.chats[0]?.run();
    await vi.waitFor(() => expect(M.S.chatOpen.api).toBe(CHAT_ID));
    expect(M.S.route).toMatchObject({ page: "project", pid: "api", view: "chat" });
  });

  it("reads a project's chats when the store does not hold the chat yet", async () => {
    const { d, M, ctx } = await synced();
    const later = wireChat({ id: "01M3CHAT00000000000000000B", projectId: "web", title: "Later" });
    d.chats.push(later);
    expect(M.chatById("web", later.id)).toBeUndefined();
    const hits = hitsOf(ctx, {
      ...golden<SearchSnapshot>("search"),
      chats: [
        {
          chatId: later.id,
          title: "Later",
          projectId: "web",
          projectName: "web-dashboard",
          lastActiveAt: later.lastActiveAt,
        },
      ],
    });
    hits.chats[0]?.run();
    await vi.waitFor(() => expect(M.S.chatOpen.web).toBe(later.id));
    expect(d.routes()).toContain("GET /v1/projects/web/chats");
  });

  it("says so when the chat is gone, and opens nothing", async () => {
    const { M, ctx } = await synced();
    const hits = hitsOf(ctx, {
      ...golden<SearchSnapshot>("search"),
      chats: [
        {
          chatId: "01M3CHAT0000000000000000ZZ",
          title: "Gone",
          projectId: "api",
          projectName: "api-gateway",
          lastActiveAt: "2026-09-30T10:00:00.000Z",
        },
      ],
    });
    M.go("home");
    hits.chats[0]?.run();
    await vi.waitFor(() =>
      expect(toasts(M)).toEqual(["Marshal cannot find that chat. It may have been removed."]),
    );
    expect(M.S.route.page).toBe("home");
  });
});
