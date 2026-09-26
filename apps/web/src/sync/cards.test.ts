import type { Card as WireCard } from "@marshal/protocol";
import { createEffect, createRoot } from "solid-js";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toDaemonProject } from "~/data/mappers/project";
import { sectionStatus } from "~/data/sections";
import type { Ctx } from "~/mock/context";
import { boardOf, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS, wireProject } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { applyCard, applyCardRemoved, applyCardSnapshot, cardsSyncer } from "./cards";

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});
const open = (options?: Parameters<typeof createFakeDaemon>[0]): FakeDaemon => {
  daemon = createFakeDaemon(options);
  return daemon;
};

/** What section S5a says once the cards are switched to the daemon. */
const onDaemon = (d: FakeDaemon) => ({
  data: d.data,
  sections: { ...sectionStatus, S5a: "daemon" as const },
});

/** A store that follows the daemon, waited for until its boards are in. */
async function synced(d: FakeDaemon) {
  const M = createTestMarshal(onDaemon(d));
  await d.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  return M;
}

const keys = (M: { S: { cards: readonly { id: string }[] } }) => M.S.cards.map((card) => card.id);
/** The topics the client last told the daemon it wants, which is what a hello carries. */
const lastTopics = (d: FakeDaemon): string[] =>
  (d.sockets.last().hellos().at(-1)?.subscribe as string[] | undefined) ?? [];
const api41 = (fields: Partial<WireCard> = {}): WireCard =>
  wireCard({
    projectId: "api",
    number: 41,
    title: "Fix token refresh on login",
    state: "working",
    ...fields,
  });

/** Tells the client that events were missed, as the daemon does after a break, so it loads again. */
function resync(d: FakeDaemon, epoch: string): void {
  d.sockets.last().push({ type: "resync", epoch, reason: "epoch-changed", seq: 0 });
}

/** One event as the stream delivers it, for the tests that call the handler directly. */
function eventOf(type: string, data: unknown) {
  return { seq: 1, topic: "project:api", type, at: "2026-09-26T12:00:00.000Z", data } as never;
}

describe("the cards section", () => {
  it("is section S5a: no topic of its own, one per project, and it reads the card events", () => {
    expect(cardsSyncer.section).toBe("S5a");
    expect(cardsSyncer.topics).toEqual([]);
    expect(cardsSyncer.projectTopics?.("api")).toEqual(["project:api"]);
    expect(cardsSyncer.projectTopics?.("web")).toEqual(["project:web"]);
    expect(cardsSyncer.onEvent).toBeTypeOf("function");
  });

  it("loads one board per project, because there is no call that lists cards across projects", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS, cards: [api41()] });
    const M = await synced(d);
    expect(d.routes()).toContain("GET /v1/projects/api/board");
    expect(d.routes()).toContain("GET /v1/projects/web/board");
    expect(d.routes()).toContain("GET /v1/projects/mobile/board");
    expect(keys(M)).toEqual(["api#41"]);
  });

  it("subscribes to each project's topic, so a change made elsewhere arrives", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS.slice(0, 2), cards: [api41()] });
    const M = await synced(d);
    await vi.waitFor(() => expect(lastTopics(d)).toContain("project:api"));
    expect(lastTopics(d)).toContain("project:web");
    d.emit("project:api", "card.moved", { card: api41({ state: "needs" }), from: "working" });
    await vi.waitFor(() => expect(M.card("api#41")?.state).toBe("needs"));
  });

  it("subscribes to a project that appears, and unsubscribes from one that is removed", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS.slice(0, 2) });
    await synced(d);
    await vi.waitFor(() => expect(lastTopics(d)).toContain("project:web"));
    d.projects.pop();
    resync(d, "01M3C0ZZZZ000000000000000C");
    await vi.waitFor(() => expect(lastTopics(d)).not.toContain("project:web"));
    d.projects.push(wireProject({ id: "later", name: "later", path: "/code/later" }));
    resync(d, "01M3C0ZZZZ000000000000000D");
    await vi.waitFor(() => expect(lastTopics(d)).toContain("project:later"));
  });

  it("draws no cards of its own while the section is still on the mock", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS, cards: [api41()] });
    const M = createTestMarshal({ data: d.data, sections: { ...sectionStatus, S5a: "mock" } });
    await d.connect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    expect(d.routes()).not.toContain("GET /v1/projects/api/board");
    expect(M.S.cards).toHaveLength(29);
  });
});

describe("applying a snapshot", () => {
  it("keeps the daemon's cards, in the daemon's order", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCardSnapshot(ctx, [
      boardOf("api", [api41({ number: 43, key: "api#43" }), api41()]),
      boardOf("web", [wireCard({ projectId: "web", number: 3, title: "Ship it" })]),
    ]);
    expect(ctx.S.cards.map((card) => card.id)).toEqual(["api#41", "api#43", "web#3"]);
  });

  it("updates a card in place, so a card that did not change is not redrawn", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCardSnapshot(ctx, [boardOf("api", [api41()])]);
    const before = ctx.S.cards[0];
    applyCardSnapshot(ctx, [boardOf("api", [api41()])]);
    expect(ctx.S.cards[0]).toBe(before);
    applyCardSnapshot(ctx, [
      boardOf("api", [api41({ title: "Rename the token", state: "review" })]),
    ]);
    expect(ctx.S.cards[0]).toBe(before);
    expect(ctx.S.cards[0]).toMatchObject({ title: "Rename the token", state: "review" });
  });

  it("gives a new card the app's own empty values for the fields the daemon has nothing to say about", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCardSnapshot(ctx, [boardOf("api", [api41()])]);
    expect(ctx.S.cards[0]).toMatchObject({
      cost: 0,
      deps: [],
      members: [],
      checklists: [],
      comments: [],
      bypass: false,
      mergePct: 0,
    });
  });

  it("leaves the fields the daemon does not own alone, so the mock's own figures survive a reload", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCardSnapshot(ctx, [boardOf("api", [api41()])]);
    const card = ctx.S.cards[0]!;
    card.cost = 4.2;
    card.members = ["ada"];
    applyCardSnapshot(ctx, [boardOf("api", [api41({ state: "review" })])]);
    expect(ctx.S.cards[0]).toMatchObject({ cost: 4.2, members: ["ada"], state: "review" });
  });

  it("forgets a card the daemon no longer lists, and everything the store kept for it", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCardSnapshot(ctx, [boardOf("api", [api41({ number: 43, key: "api#43" })])]);
    ctx.S.openId = "api#43";
    ctx.S.focusId = "api#43";
    ctx.S.chat["api#43"] = [];
    ctx.S.act["api#43"] = [];
    applyCardSnapshot(ctx, [boardOf("api", [])]);
    expect(ctx.S.cards).toEqual([]);
    expect(ctx.S.openId).toBeNull();
    expect(ctx.S.focusId).toBeNull();
    expect(ctx.S.chat["api#43"]).toBeUndefined();
    expect(ctx.S.act["api#43"]).toBeUndefined();
  });

  it("keeps the cards of a project whose board was not part of the answer", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCardSnapshot(ctx, [boardOf("api", [api41()]), boardOf("web", [])]);
    applyCardSnapshot(ctx, [boardOf("web", [])]);
    expect(ctx.S.cards.map((card) => card.id)).toEqual(["api#41"]);
  });
});

describe("applying one card", () => {
  it("adds a card that is new and keeps the board's order", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCard(ctx, api41({ number: 43, key: "api#43" }));
    applyCard(ctx, api41());
    expect(ctx.S.cards.map((card) => card.id)).toEqual(["api#41", "api#43"]);
  });

  it("changes a card that is already there in place, and a repeat of the event changes nothing", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCard(ctx, api41());
    const card = ctx.S.cards[0];
    applyCard(ctx, api41({ title: "Fix login" }));
    applyCard(ctx, api41({ title: "Fix login" }));
    expect(ctx.S.cards[0]).toBe(card);
    expect(ctx.S.cards).toHaveLength(1);
    expect(ctx.S.cards[0]?.title).toBe("Fix login");
  });

  it("removes a card by its key, and does nothing when it is already gone", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCard(ctx, api41());
    applyCardRemoved(ctx, "api#41");
    expect(ctx.S.cards).toEqual([]);
    expect(() => applyCardRemoved(ctx, "api#41")).not.toThrow();
  });
});

describe("the card events", () => {
  it("takes the card out of a card.created, a card.updated, and a card.moved", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    for (const type of ["card.created", "card.updated", "card.moved"]) {
      cardsSyncer.onEvent?.(
        ctx,
        eventOf(type, { card: api41({ title: `from ${type}` }), from: "working" }),
      );
      expect(ctx.S.cards[0]?.title).toBe(`from ${type}`);
    }
  });

  it("forgets the card a card.deleted names by its key, which is what the store is keyed by", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCard(ctx, api41());
    cardsSyncer.onEvent?.(
      ctx,
      eventOf("card.deleted", { cardId: api41().id, key: "api#41", projectId: "api" }),
    );
    expect(ctx.S.cards).toEqual([]);
  });

  it("ignores the events of the other sections, and a payload that is not a card", () => {
    const ctx: Ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    for (const [type, data] of [
      ["project.created", {}],
      ["session.state_changed", {}],
      ["card.moved", {}],
      ["card.deleted", {}],
      ["card.deleted", { key: 41 }],
      ["card.moved", { card: "not a card" }],
    ] as const) {
      cardsSyncer.onEvent?.(ctx, eventOf(type, data));
    }
    expect(ctx.S.cards).toEqual([]);
  });

  it("arrives on the project's topic and lands in the store the app draws", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS.slice(0, 1), cards: [api41()] });
    const M = await synced(d);
    d.emit("project:api", "card.created", {
      card: api41({ number: 42, key: "api#42", title: "New" }),
    });
    await vi.waitFor(() => expect(keys(M)).toEqual(["api#41", "api#42"]));
    d.emit("project:api", "card.deleted", { cardId: api41().id, key: "api#41", projectId: "api" });
    await vi.waitFor(() => expect(keys(M)).toEqual(["api#42"]));
  });
});

describe("a card's session (section S7c)", () => {
  it("draws asleep and waking from the board the daemon answers, and no card is asleep without it", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCardSnapshot(ctx, [
      boardOf("api", [
        api41({ session: "awake" }),
        api41({ number: 42, key: "api#42", session: "asleep" }),
        api41({ number: 43, key: "api#43", session: "waking" }),
        api41({ number: 44, key: "api#44", session: null, state: "backlog" }),
      ]),
    ]);
    const by = (key: string) => ctx.S.cards.find((card) => card.id === key);
    expect(by("api#41")).toMatchObject({ session: "awake", asleep: false, waking: false });
    expect(by("api#42")).toMatchObject({ session: "asleep", asleep: true, waking: false });
    expect(by("api#43")).toMatchObject({ session: "waking", asleep: true, waking: true });
    expect(by("api#44")).toMatchObject({ session: null, asleep: false, waking: false });
  });

  it("follows every card.updated, in place, and a repeat of one changes nothing", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.cards = [];
    applyCard(ctx, api41({ session: "awake" }));
    const card = ctx.S.cards[0]!;
    const seen: string[] = [];
    const stop = createRoot((dispose) => {
      createEffect(() => seen.push(`${card.session}:${card.asleep}:${card.waking}`));
      return dispose;
    });
    for (const session of ["asleep", "asleep", "waking", "awake", "awake"] as const) {
      cardsSyncer.onEvent?.(ctx, eventOf("card.updated", { card: api41({ session }) }));
    }
    stop();
    expect(ctx.S.cards[0]).toBe(card);
    // Each distinct state is drawn once: the repeats redraw nothing.
    expect(seen).toEqual([
      "awake:false:false",
      "asleep:true:false",
      "waking:true:true",
      "awake:false:false",
    ]);
  });

  it("is still asleep after the page is reloaded, and after the daemon restarts", async () => {
    const d = open({
      projects: PROTOTYPE_PROJECTS.slice(0, 1),
      cards: [
        api41({ session: "asleep", paused: true }),
        api41({ number: 42, key: "api#42", session: "waking" }),
      ],
    });
    const first = await synced(d);
    expect(first.card("api#41")).toMatchObject({ asleep: true, waking: false });
    expect(first.card("api#42")).toMatchObject({ asleep: true, waking: true });
    // A reload of the page is a store made from nothing that reads the boards again.
    const reloaded = await synced(d);
    expect(reloaded.card("api#41")).toMatchObject({ asleep: true, waking: false, paused: true });
    expect(reloaded.card("api#42")).toMatchObject({ asleep: true, waking: true });
    // A restart of the daemon sends a Resync, and the store reads the boards again. What the daemon
    // stored is what comes back, so the card that was asleep still is, and the one that woke is awake.
    d.cards[1]!.session = "awake";
    resync(d, "01M3C0ZZZZ000000000000000B");
    await vi.waitFor(() =>
      expect(first.card("api#42")).toMatchObject({ asleep: false, waking: false }),
    );
    expect(first.card("api#41")).toMatchObject({ asleep: true, paused: true });
  });
});

describe("which cards are awake (section S7c)", () => {
  /** A card for each stored session state, and one that never had a session. */
  const CARDS = [
    api41({ number: 1, key: "api#1", session: "awake" }),
    api41({ number: 2, key: "api#2", session: "working" }),
    api41({ number: 3, key: "api#3", session: "asleep", state: "review" }),
    api41({ number: 4, key: "api#4", session: "waking", state: "review" }),
    api41({ number: 5, key: "api#5", session: "stopped", state: "review" }),
    api41({ number: 6, key: "api#6", session: null, state: "review" }),
    api41({ number: 7, key: "api#7", session: "waiting-approval", state: "needs" }),
    api41({ number: 8, key: "api#8", session: null, state: "backlog" }),
  ];
  const AWAKE = ["api#1", "api#2", "api#7"];

  it("is decided by the session the daemon sent, and not by the column the card is in", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS.slice(0, 1), cards: CARDS });
    const M = await synced(d);
    expect(
      M.awake("api")
        .map((card) => card.id)
        .sort(),
    ).toEqual(AWAKE);
    // A card in review with a stopped session, or with none, is not awake, which the column alone
    // used to say it was.
    expect(M.isAwake(M.card("api#5")!)).toBe(false);
    expect(M.isAwake(M.card("api#6")!)).toBe(false);
    expect(M.isAwake(M.card("api#4")!)).toBe(false);
  });

  it("gives the same count as the daemon's own badge for the project", async () => {
    // The daemon counts the live sessions of a project's cards: the ones with a running process, which
    // are the awake, working, waiting, and sleep-warning ones. A card that is asleep, waking, stopped,
    // or has no session has no process, so the daemon's badge is this many.
    const d = open({
      projects: PROTOTYPE_PROJECTS.slice(0, 1).map((project) => ({
        ...project,
        badges: { needs: 1, awake: AWAKE.length },
      })),
      cards: CARDS,
    });
    const M = await synced(d);
    const badge = toDaemonProject(d.projects[0]!).awake;
    expect(M.awake("api")).toHaveLength(badge);
    expect(M.S.cards.filter((card) => M.isAwake(card))).toHaveLength(badge);
  });

  it("hears a sleep and a wake on the project's topic, with no card open", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS.slice(0, 1), cards: CARDS.slice(0, 2) });
    const M = await synced(d);
    expect(M.S.openId).toBeNull();
    d.emit("project:api", "card.updated", { card: { ...CARDS[0]!, session: "asleep" } });
    await vi.waitFor(() => expect(M.awake("api").map((card) => card.id)).toEqual(["api#2"]));
    d.emit("project:api", "card.updated", { card: { ...CARDS[0]!, session: "waking" } });
    await vi.waitFor(() => expect(M.card("api#1")?.waking).toBe(true));
    expect(M.awake("api").map((card) => card.id)).toEqual(["api#2"]);
    d.emit("project:api", "card.updated", { card: { ...CARDS[0]!, session: "awake" } });
    await vi.waitFor(() =>
      expect(
        M.awake("api")
          .map((card) => card.id)
          .sort(),
      ).toEqual(["api#1", "api#2"]),
    );
  });

  it("keeps the mock's own rule for a card the mock made, which has no session", () => {
    const M = createTestMarshal({ sections: { ...sectionStatus, S5a: "mock" as const } });
    const mock = M.S.cards.find((card) => card.state === "working")!;
    expect(mock.session).toBeUndefined();
    expect(M.isAwake(mock)).toBe(true);
    expect(M.isAwake({ ...mock, asleep: true })).toBe(false);
    expect(M.isAwake({ ...mock, state: "backlog" })).toBe(false);
    expect(M.isAwake({ ...mock, state: "done" })).toBe(false);
  });
});
