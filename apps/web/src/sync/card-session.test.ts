import {
  type Event,
  EventTypeSessionOutput,
  EventTypeSessionStateChanged,
  EventTypeSessionToolCall,
} from "@marshal/protocol";
import { createRoot } from "solid-js";
import { afterEach, describe, expect, it, vi } from "vitest";
import { isDaemon, sectionStatus } from "~/data/sections";
import { historyRow, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { applyCardSessionEvent, followOpenCard, readOpenCard, sendToCard } from "./card-session";

/** The card the daemon holds, and the key the store keeps it under. */
const CARD = wireCard({ projectId: "api", number: 41, title: "Fix token refresh on login" });
const KEY = CARD.key;

/** The chat the daemon recorded for it: newest first, as its index keeps it. */
const history = [
  historyRow(CARD.id, { id: "m3", kind: "agent", text: "Working on it", seq: 3 }),
  historyRow(CARD.id, { id: "m2", kind: "system", text: "Session started", seq: 2 }),
  historyRow(
    CARD.id,
    { id: "m1", kind: "user", text: "Fix the token refresh", seq: 1 },
    {
      id: "a1",
      kind: "command",
      seq: 1,
      at: "2026-09-26T12:00:00.000Z",
      text: "pnpm test",
      result: "Passed",
      state: "ok",
    },
  ),
];

const SECTIONS = { ...sectionStatus, S8a: "daemon" as const, S10: "daemon" as const };

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

/** A store that follows a fake daemon holding one card and its history, waited for until it is ready. */
async function storeWithHistory() {
  daemon = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards: [CARD], history });
  const M = createTestMarshal({ data: daemon.data, sections: SECTIONS });
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  return { M, ctx: contextOf(M), d: daemon };
}

describe("reading the open card's history", () => {
  it("puts the daemon's chat in the store oldest first, which is the order the screen draws it in", async () => {
    const { ctx, d } = await storeWithHistory();
    const card = ctx.S.cards.find((one) => one.id === KEY)!;
    // The card is open, which is when its history is read.
    ctx.S.openId = KEY;
    await readOpenCard(ctx, d.data.api, card, KEY);
    expect(ctx.S.chat[KEY]?.map((one) => one.id)).toEqual(["m1", "m2", "m3"]);
    expect(ctx.S.chat[KEY]?.[0]).toEqual({ id: "m1", k: "user", text: "Fix the token refresh" });
  });

  it("puts the card's activity in the store with the store's own words for it", async () => {
    const { ctx, d } = await storeWithHistory();
    const card = ctx.S.cards.find((one) => one.id === KEY)!;
    ctx.S.openId = KEY;
    await readOpenCard(ctx, d.data.api, card, KEY);
    expect(ctx.S.act[KEY]?.map((one) => one.id)).toEqual(["a1"]);
    expect(ctx.S.act[KEY]?.[0]).toMatchObject({ kind: "command", text: "pnpm test", st: "ok" });
  });

  it("reads nothing while the section is still the mock's, so the store keeps what it has", async () => {
    const { ctx, d } = await storeWithHistory();
    const mockStore = createTestMarshal({
      data: d.data,
      sections: { ...SECTIONS, S8a: "mock", S10: "mock" },
    });
    const ctxMock = contextOf(mockStore);
    const seeded = ctxMock.S.chat[KEY];
    ctxMock.S.openId = KEY;
    await readOpenCard(ctxMock, d.data.api, { daemonId: CARD.id }, KEY);
    expect(ctxMock.S.chat[KEY]).toBe(seeded);
    expect(isDaemon("S8a", { ...SECTIONS, S8a: "mock" })).toBe(false);
    expect(ctx.S.chat[KEY]).toBeUndefined();
  });
});

describe("following the open card", () => {
  it("subscribes to that card's own topic when it opens, and lets go when it closes", async () => {
    const { ctx, d } = await storeWithHistory();
    const topics: string[] = [];
    const stream = {
      subscribe: (list: readonly string[]) => topics.push(...list.map((one) => `+${one}`)),
      unsubscribe: (list: readonly string[]) => topics.push(...list.map((one) => `-${one}`)),
    };
    const dispose = createRoot((stop) => {
      followOpenCard(ctx, d.data.api, stream);
      return stop;
    });
    // Nothing is open yet, so the stream is left alone.
    expect(topics).toEqual([]);

    ctx.S.openId = KEY;
    await vi.waitFor(() => expect(topics).toEqual([`+card:${CARD.id}`]));

    ctx.S.openId = null;
    await vi.waitFor(() => expect(topics).toEqual([`+card:${CARD.id}`, `-card:${CARD.id}`]));
    dispose();
  });
});

describe("a card's own live events", () => {
  const output = (text: string): Event => ({
    seq: 1,
    topic: `card:${CARD.id}`,
    type: EventTypeSessionOutput,
    at: "2026-09-26T12:00:01.000Z",
    data: { cardId: CARD.id, kind: "message", text },
  });
  const stateChanged = (state: string): Event => ({
    seq: 3,
    topic: `card:${CARD.id}`,
    type: EventTypeSessionStateChanged,
    at: "2026-09-26T12:00:03.000Z",
    data: { cardId: CARD.id, sessionId: "s1", state },
  });
  const toolCall = (title: string, status: string): Event => ({
    seq: 2,
    topic: `card:${CARD.id}`,
    type: EventTypeSessionToolCall,
    at: "2026-09-26T12:00:02.000Z",
    data: { cardId: CARD.id, kind: "tool_call", toolCall: { id: "call-1", title, status } },
  });

  it("joins the words of one answer into the message that is still streaming", async () => {
    const { ctx } = await storeWithHistory();
    applyCardSessionEvent(ctx, output("Hello"));
    applyCardSessionEvent(ctx, output(" world"));
    expect(ctx.S.chat[KEY]).toEqual([
      { id: expect.stringContaining("s"), k: "agent", text: "Hello world", streaming: true },
    ]);
  });

  it("starts a new message once the last answer has finished", async () => {
    const { ctx } = await storeWithHistory();
    applyCardSessionEvent(ctx, output("One"));
    const first = ctx.S.chat[KEY]?.at(-1);
    if (first?.k === "agent") first.streaming = false;
    applyCardSessionEvent(ctx, output("Two"));
    expect(ctx.S.chat[KEY]?.map((one) => (one.k === "agent" ? one.text : one.k))).toEqual([
      "One",
      "Two",
    ]);
  });

  it("shows a tool call and then keeps it up to date, by its own id, in the words the wire uses", async () => {
    const { ctx } = await storeWithHistory();
    applyCardSessionEvent(ctx, toolCall("Run tests", "in_progress"));
    const running = ctx.S.chat[KEY]?.filter((one) => one.k === "tool") ?? [];
    expect(running[0]).toMatchObject({ action: "Run tests", st: "running" });
    applyCardSessionEvent(ctx, toolCall("Run tests", "completed"));
    const tools = ctx.S.chat[KEY]?.filter((one) => one.k === "tool") ?? [];
    expect(tools).toHaveLength(1);
    expect(tools[0]).toMatchObject({ action: "Run tests", st: "ok" });
  });

  it("draws a call that failed as failed, and one that starts pending as running", async () => {
    const { ctx } = await storeWithHistory();
    applyCardSessionEvent(ctx, toolCall("Lint", "pending"));
    expect(ctx.S.chat[KEY]?.at(-1)).toMatchObject({ st: "running" });
    applyCardSessionEvent(ctx, toolCall("Lint", "failed"));
    expect(ctx.S.chat[KEY]?.at(-1)).toMatchObject({ st: "fail" });
  });

  it("ends the words of a turn when a tool call starts after them and when the session leaves the turn", async () => {
    const { ctx } = await storeWithHistory();
    applyCardSessionEvent(ctx, output("Let me look."));
    applyCardSessionEvent(ctx, toolCall("Read README.md", "in_progress"));
    expect(ctx.S.chat[KEY]?.[0]).toMatchObject({ k: "agent", streaming: false });
    applyCardSessionEvent(ctx, output("Done."));
    expect(ctx.S.chat[KEY]?.at(-1)).toMatchObject({ k: "agent", streaming: true });
    // The session goes back to waiting: the last answer is finished, and the next one is its own message.
    applyCardSessionEvent(ctx, stateChanged("awake"));
    expect(ctx.S.chat[KEY]?.at(-1)).toMatchObject({ streaming: false });
    applyCardSessionEvent(ctx, output("Next turn."));
    expect(ctx.S.chat[KEY]?.filter((one) => one.k === "agent")).toHaveLength(3);
  });

  it("ignores an event about a card the store does not have", async () => {
    const { ctx } = await storeWithHistory();
    applyCardSessionEvent(ctx, {
      seq: 3,
      topic: "card:01M3CARDNOPE",
      type: EventTypeSessionOutput,
      at: "2026-09-26T12:00:03.000Z",
      data: { cardId: "01M3CARDNOPE", kind: "message", text: "hi" },
    });
    expect(ctx.S.chat[KEY]).toBeUndefined();
  });
});

describe("sending the person's own message", () => {
  it("shows it at once and asks the daemon for it", async () => {
    const { ctx, d } = await storeWithHistory();
    const card = ctx.S.cards.find((one) => one.id === KEY)!;
    ctx.S.openId = KEY;
    expect(await sendToCard(ctx, d.data.api, KEY, "  Fix it  ")).toBe(true);
    expect(ctx.S.chat[KEY]?.at(-1)).toMatchObject({ k: "user", text: "Fix it" });
    expect(d.bodies(`POST /v1/cards/${card.daemonId}/messages`)).toEqual([{ text: "Fix it" }]);
  });

  it("takes the words back out, and only them, when a chat that already has messages is refused", async () => {
    const { ctx, d } = await storeWithHistory();
    const card = ctx.S.cards.find((one) => one.id === KEY)!;
    ctx.S.openId = KEY;
    await readOpenCard(ctx, d.data.api, card, KEY);
    d.refuseNext(`POST /v1/cards/${card.daemonId}/messages`, 409, "conflict", "Not running.");
    expect(await sendToCard(ctx, d.data.api, KEY, "Hello")).toBe(false);
    expect(ctx.S.chat[KEY]?.map((one) => one.id)).toEqual(["m1", "m2", "m3"]);
  });

  it("takes the words back out and says why when the daemon refuses", async () => {
    const { ctx, d } = await storeWithHistory();
    const card = ctx.S.cards.find((one) => one.id === KEY)!;
    d.refuseNext(
      `POST /v1/cards/${card.daemonId}/messages`,
      409,
      "conflict",
      "The session is not running.",
    );
    expect(await sendToCard(ctx, d.data.api, KEY, "Hello")).toBe(false);
    expect(ctx.S.chat[KEY] ?? []).toEqual([]);
    expect(ctx.S.toasts.map((toast) => toast.msg)).toContain("The session is not running.");
  });
});
