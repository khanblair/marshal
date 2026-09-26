import {
  type EventType,
  EventTypeCardMoved,
  type Card as WireCard,
  type Event as WireEvent,
} from "@marshal/protocol";
import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import type { Marshal } from "~/mock";
import type { Ctx } from "~/mock/context";
import type { Card } from "~/mock/types";
import { cardId, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import {
  createCard,
  deleteCard,
  fork,
  moveCard,
  quickAdd,
  rename,
  setSetting,
  start,
  stop,
} from "./card-actions";
import { cardsSyncer } from "./cards";

/** The sections with the cards on the daemon, which is what the cutover of S5a will say. */
const CARDS_ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const };

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

const api41 = (fields: Partial<WireCard> = {}): WireCard =>
  wireCard({
    projectId: "api",
    number: 41,
    title: "Fix token refresh on login",
    state: "working",
    ...fields,
  });

/** The address of one card's route, as the client sends it: the key is escaped in the path. */
/**
 * The route a card key means. A card's key (`api#41`) is what the screens and the store use, and the
 * daemon's card routes take its own opaque id instead, which the fixture makes from the number.
 */
const route = (method: string, key: string, action = ""): string => {
  const number = Number(key.split("#")[1] ?? "0");
  return `${method} /v1/cards/${encodeURIComponent(cardId(number))}${action ? `/${action}` : ""}`;
};

interface Fixture {
  M: Marshal;
  ctx: Ctx;
  d: FakeDaemon;
}

/** A store that follows a fake daemon, waited for until its boards are in and no more are loading. */
async function setup(cards: readonly WireCard[] = [api41()]): Promise<Fixture> {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards });
  daemon = d;
  const M = createTestMarshal({ data: d.data, sections: CARDS_ON_DAEMON });
  await d.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  await settled(d);
  return { M, ctx: contextOf(M), d };
}

/**
 * Waits until the app has stopped asking for boards. The stream's first Resync makes it load once
 * more after `ready`, and that second snapshot landing on top of a change a test just made would
 * put the card back the way the daemon had it before the change.
 */
async function settled(d: FakeDaemon): Promise<void> {
  let last = -1;
  await vi.waitFor(() => {
    const loads = d.routes().filter((one) => one.endsWith("/board")).length;
    const steady = loads > 0 && loads === last;
    last = loads;
    expect(steady).toBe(true);
  });
}

function storeCard(M: Marshal, id: string): Card {
  const card = M.card(id);
  if (!card) throw new Error(`the store has no card ${id}`);
  return card;
}

/** The daemon's own copy of a card, which is what its events carry. */
function daemonCard(d: FakeDaemon, key: string): WireCard {
  const card = d.cards.find((one) => one.key === key);
  if (!card) throw new Error(`the fake daemon has no card ${key}`);
  return card;
}

/** One event as the stream delivers it, for the tests that apply the daemon's own a second time. */
const eventOf = (type: EventType, data: unknown): WireEvent => ({
  seq: 1,
  topic: "project:api",
  type,
  at: "2026-09-26T12:00:00.000Z",
  data,
});

const toasts = (M: Marshal): string[] => M.S.toasts.map((toast) => toast.msg);

describe("moveCard", () => {
  it("lands the drag, keeps the daemon's answer, and the event that follows changes nothing", async () => {
    const { M, ctx, d } = await setup();
    const before = storeCard(M, "api#41");
    expect(await moveCard(ctx, "api#41", "review")).toBeNull();
    expect(d.bodies(route("POST", "api#41", "move"))).toEqual([{ state: "review" }]);
    expect(before.state).toBe("review");
    // The daemon published card.moved for the same change. Applying it twice changes nothing.
    const moved = daemonCard(d, "api#41");
    expect(moved.state).toBe("review");
    cardsSyncer.onEvent?.(ctx, eventOf(EventTypeCardMoved, { card: moved, from: "working" }));
    cardsSyncer.onEvent?.(ctx, eventOf(EventTypeCardMoved, { card: moved, from: "working" }));
    expect(M.card("api#41")).toBe(before);
    expect(before.state).toBe("review");
  });

  it("snaps a refused move back with the daemon's sentence, leaving the card exactly as it was", async () => {
    const { M, ctx, d } = await setup();
    const before = { ...storeCard(M, "api#41") };
    const why = await moveCard(ctx, "api#41", "done");
    expect(why).toBe("Only a merged pull request moves a card to Done.");
    expect(d.bodies(route("POST", "api#41", "move"))).toEqual([{ state: "done" }]);
    // The drag landed. The toast is what proves the snap-back itself ran: a board snapshot could
    // put the state back on its own, but only the refusal says the daemon's sentence.
    expect(storeCard(M, "api#41").state).toBe("done");
    await vi.waitFor(() =>
      expect(toasts(M)).toEqual(["Only a merged pull request moves a card to Done."]),
    );
    expect(storeCard(M, "api#41").state).toBe("working");
    expect(storeCard(M, "api#41")).toEqual(before);
  });

  it("asks for nothing when the card is already in that column or is not there", async () => {
    const { ctx, d } = await setup();
    expect(await moveCard(ctx, "api#41", "working")).toBeNull();
    expect(await moveCard(ctx, "api#9999", "review")).toBeNull();
    expect(d.routes().filter((one) => one.endsWith("/move"))).toEqual([]);
  });
});

describe("rename and setSetting", () => {
  it("renames through the daemon's card edit, trimming what the field holds", async () => {
    const { M, ctx, d } = await setup();
    expect(await rename(ctx, "api#41", "  Fix the token refresh  ")).toBe(true);
    expect(d.bodies(route("PATCH", "api#41"))).toEqual([{ title: "Fix the token refresh" }]);
    expect(M.card("api#41")?.title).toBe("Fix the token refresh");
  });

  it("keeps the old name without asking when the new one is empty or the same", async () => {
    const { ctx, d } = await setup();
    expect(await rename(ctx, "api#41", "   ")).toBe(false);
    expect(await rename(ctx, "api#41", "Fix token refresh on login")).toBe(false);
    expect(await rename(ctx, "api#9999", "New")).toBe(false);
    expect(d.routes().filter((one) => one.startsWith("PATCH"))).toEqual([]);
  });

  it.each([
    ["agent", "Codex", { agent: "codex", model: "gpt-5-codex", thinking: "" }],
    ["role", "Tester", { role: "Tester" }],
    ["model", "gpt-5-mini", { model: "gpt-5-mini" }],
    ["think", "Extra high", { thinking: "extra-high" }],
    ["perm", "Plan only", { permissionMode: "plan" }],
    // An agent change carries the model that agent starts its cards on and the thinking rule that
    // goes with it, which is what the mock's own setting did (see `thinkingFor`).
  ] as const)("sends the setting %s as the wire asks for %o", async (key, value, body) => {
    const { ctx, d } = await setup();
    expect(await setSetting(ctx, "api#41", key, value)).toBe(true);
    expect(d.bodies(route("PATCH", "api#41"))).toEqual([body]);
  });

  it("shows the daemon's sentence and keeps the old title when it refuses", async () => {
    const { M, ctx, d } = await setup();
    d.refuseNext(
      route("PATCH", "api#41"),
      400,
      "invalid_argument",
      "Card titles can have at most 200 characters, and cannot be empty.",
    );
    expect(await rename(ctx, "api#41", "x".repeat(300))).toBe(false);
    expect(M.card("api#41")?.title).toBe("Fix token refresh on login");
    expect(toasts(M)).toEqual([
      "Card titles can have at most 200 characters, and cannot be empty.",
    ]);
  });
});

describe("deleteCard", () => {
  it("asks first, and takes the card out of the store only when the person says yes", async () => {
    const { M, ctx, d } = await setup();
    expect(await deleteCard(ctx, "api#41")).toBe(true);
    // Nothing has been asked of the daemon yet: the confirmation is up, with the mock's own words.
    expect(M.S.dialog).toMatchObject({
      title: "Delete card",
      action: "Delete card",
      destructive: true,
      message: "This deletes api-gateway #41 and its notes.",
    });
    expect(d.routes().filter((one) => one.startsWith("DELETE"))).toEqual([]);
    ctx.S.dialog?.run();
    await vi.waitFor(() => expect(M.card("api#41")).toBeUndefined());
    expect(M.S.cards).toEqual([]);
    expect(toasts(M)).toEqual(["Card deleted"]);
    expect(d.routes().filter((one) => one.startsWith("DELETE"))).toEqual([
      route("DELETE", "api#41"),
    ]);
  });

  it("is harmless when the card is already gone", async () => {
    const { M, ctx } = await setup();
    expect(await deleteCard(ctx, "api#41")).toBe(true);
    ctx.S.dialog?.run();
    await vi.waitFor(() => expect(M.card("api#41")).toBeUndefined());
    expect(await deleteCard(ctx, "api#41")).toBe(false);
  });
});

describe("fork", () => {
  it("lands the copy the daemon numbered and named", async () => {
    const { M, ctx, d } = await setup();
    expect(await fork(ctx, "api#41")).toBe(true);
    expect(d.routes()).toContain(route("POST", "api#41", "fork"));
    expect(M.card("api#42")?.title).toBe("Fix token refresh on login (fork)");
    expect(M.card("api#42")?.p).toBe("api");
    expect(toasts(M)).toEqual(["Card forked"]);
  });
});

describe("quickAdd and createCard", () => {
  it("adds a quick card in the column it was typed in, and starts it when the column does", async () => {
    const { M, ctx, d } = await setup([]);
    expect(await quickAdd(ctx, "api", "planning", "  Wire up metrics  ")).toBe("api#1");
    expect(d.bodies("POST /v1/projects/api/cards")).toEqual([
      { title: "Wire up metrics", permissionMode: "plan", startState: "planning" },
    ]);
    expect(M.card("api#1")?.p).toBe("api");
    expect(M.card("api#1")?.title).toBe("Wire up metrics");
    expect(toasts(M)).toEqual(["Card created and started"]);
  });

  it("adds a backlog card that is not started, and one for a lane", async () => {
    const { ctx, d } = await setup([]);
    expect(await quickAdd(ctx, "api", "backlog", "Notes")).toBe("api#1");
    expect(
      await quickAdd(ctx, "api", "backlog", "Lane card", { role: "Tester", pkg: "pkg/a" }),
    ).toBe("api#2");
    expect(d.bodies("POST /v1/projects/api/cards")).toEqual([
      { title: "Notes", permissionMode: "auto-edits", startState: "backlog" },
      {
        title: "Lane card",
        role: "Tester",
        package: "pkg/a",
        permissionMode: "auto-edits",
        startState: "backlog",
      },
    ]);
  });

  it("adds nothing when the quick-add field is empty", async () => {
    const { ctx, d } = await setup([]);
    expect(await quickAdd(ctx, "api", "backlog", "   ")).toBeNull();
    expect(d.routes()).not.toContain("POST /v1/projects/api/cards");
  });

  it("creates the card drafted in the New card dialog, in the project the app is on", async () => {
    const { M, ctx, d } = await setup([]);
    M.go("project", "api", "board");
    ctx.S.newCard = {
      title: "Audit logs",
      body: "Add audit logs",
      template: "Plan first",
      role: "Worker",
      agent: "Claude Code",
      start: true,
    };
    expect(await createCard(ctx)).toBe("api#1");
    expect(d.bodies("POST /v1/projects/api/cards")).toEqual([
      {
        title: "Audit logs",
        body: "Add audit logs",
        role: "Worker",
        agent: "claude",
        // The agent's own default model - Claude Code's first model in this test's catalog -
        // which is what the mock's dialog wrote on the card too.
        model: "sonnet",
        permissionMode: "plan",
        startState: "planning",
      },
    ]);
    expect(M.card("api#1")?.title).toBe("Audit logs");
    expect(M.S.newCard).toBeNull();
  });

  it("keeps the draft when the daemon refuses, so the dialog stays open", async () => {
    const { M, ctx, d } = await setup([]);
    M.go("project", "api", "board");
    ctx.S.newCard = {
      title: "Audit logs",
      body: "",
      template: "Blank",
      role: "Worker",
      agent: "Claude Code",
      start: false,
    };
    d.refuseNext("POST /v1/projects/api/cards", 400, "invalid_argument", "Give the card a name.");
    expect(await createCard(ctx)).toBeNull();
    expect(M.S.newCard).not.toBeNull();
    expect(toasts(M)).toEqual(["Give the card a name."]);
  });
});

describe("start and stop", () => {
  it("starts a card's session and stops it again", async () => {
    const { M, ctx, d } = await setup();
    expect(await start(ctx, "api#41")).toBe(true);
    expect(d.routes()).toContain(route("POST", "api#41", "start"));
    expect(toasts(M)).toEqual(["Card started"]);
    expect(await stop(ctx, "api#41")).toBe(true);
    expect(d.routes()).toContain(route("POST", "api#41", "stop"));
  });

  it("asks for nothing for a card the store does not have", async () => {
    const { ctx, d } = await setup();
    expect(await start(ctx, "api#9999")).toBe(false);
    expect(await stop(ctx, "api#9999")).toBe(false);
    expect(d.routes().filter((one) => one.includes("/start") || one.includes("/stop"))).toEqual([]);
  });
});

describe("a store with no daemon", () => {
  it("says so plainly, changes nothing, and throws nothing", async () => {
    const M = createTestMarshal({ sections: CARDS_ON_DAEMON });
    const ctx = contextOf(M);
    const cards = M.S.cards.length;
    await expect(moveCard(ctx, "api#41", "review")).resolves.toBe(NOT_CONNECTED);
    await expect(rename(ctx, "api#41", "New title")).resolves.toBe(false);
    await expect(setSetting(ctx, "api#41", "think", "High")).resolves.toBe(false);
    // Delete asks before it acts, so it answers that the question is up; the refusal is said when
    // the person answers it, and the card is left alone.
    await expect(deleteCard(ctx, "api#41")).resolves.toBe(true);
    ctx.S.dialog?.run();
    await vi.waitFor(() => expect(toasts(M)).toContain(NOT_CONNECTED));
    expect(M.card("api#41")).toBeDefined();
    await expect(fork(ctx, "api#41")).resolves.toBe(false);
    await expect(quickAdd(ctx, "api", "backlog", "New")).resolves.toBeNull();
    await expect(start(ctx, "api#41")).resolves.toBe(false);
    await expect(stop(ctx, "api#41")).resolves.toBe(false);
    expect(M.S.cards).toHaveLength(cards);
    // The store keeps only the newest few toasts, so what is read back is the tail of them.
    expect(toasts(M)).toHaveLength(3);
    expect(new Set(toasts(M))).toEqual(new Set([NOT_CONNECTED]));
  });
});
