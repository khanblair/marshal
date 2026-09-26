import {
  EventTypeSessionOutput,
  EventTypeSessionStateChanged,
  EventTypeSessionToolCall,
  type Event as WireEvent,
} from "@marshal/protocol";
import { createRoot } from "solid-js";
import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import { chatMessageRow, wireChat } from "~/testing/fake-chats";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import {
  applyChatSessionEvent,
  followOpenChat,
  readOpenChat,
  retryChat,
  sendToChat,
} from "./chat-session";

// Section S17, the message side (docs/backend-checklist.md B2.10): the chats are the daemon's, and so
// is what is said in them.
const ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const, S17: "daemon" as const };

const CHAT = wireChat({
  id: "01M3CHAT00000000000000A001",
  projectId: "api",
  title: "Plan the export",
});
const OTHER = wireChat({
  id: "01M3CHAT00000000000000A002",
  projectId: "api",
  title: "Second chat",
});
const FRESH = wireChat({ id: "01M3CHAT00000000000000A003", projectId: "api", title: "New chat" });
const ARCHIVED = wireChat({
  id: "01M3CHAT00000000000000A004",
  projectId: "api",
  title: "Old plan",
  archivedAt: "2026-09-25T10:00:00.000Z",
});

/** What the daemon recorded for CHAT: a question, a tool call, and the answer. Newest is seq 3. */
const HISTORY = [
  chatMessageRow(CHAT.id, { id: "m1", kind: "user", seq: 1, text: "What is blocked?" }),
  chatMessageRow(CHAT.id, {
    id: "m2",
    kind: "tool",
    seq: 2,
    tool: {
      id: "call-1",
      title: "List the cards",
      toolKind: "other",
      state: "ok",
      hasDetail: true,
    },
  }),
  chatMessageRow(CHAT.id, {
    id: "m3",
    kind: "agent",
    seq: 3,
    text: "Nothing is blocked right now.",
  }),
];

/** The read of a chat's history, as the fake daemon names it: the first page asks for the newest fifty. */
const routeOf = (chat: { id: string }): string => `GET /v1/chats/${chat.id}/messages?limit=50`;
const sendRoute = (chat: { id: string }): string => `POST /v1/chats/${chat.id}/messages`;

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

/** A store that follows a fake daemon holding the chats above, waited for until it stopped asking for lists. */
async function setup(options: Parameters<typeof createFakeDaemon>[0] = {}) {
  const d = createFakeDaemon({
    projects: PROTOTYPE_PROJECTS,
    chats: [CHAT, OTHER, FRESH, ARCHIVED],
    chatMessages: HISTORY,
    ...options,
  });
  daemon = d;
  const M = createTestMarshal({ data: d.data, sections: ON_DAEMON });
  await d.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  // The stream's first Resync makes the app load once more after `ready`.
  let last = -1;
  await vi.waitFor(() => {
    const loads = d.routes().filter((one) => one.endsWith("/chats")).length;
    const steady = loads > 0 && loads === last;
    last = loads;
    expect(steady).toBe(true);
  });
  // The Chats view of the project is what is on screen.
  M.go("project", "api", "chat");
  return { M, ctx: contextOf(M), d, chat: (one = CHAT) => M.chatById("api", one.id)! };
}

/** The topics the client last told the daemon it wants. */
const lastTopics = (d: FakeDaemon): string[] =>
  (d.sockets.last().hellos().at(-1)?.subscribe as string[] | undefined) ?? [];

const messageCalls = (d: FakeDaemon, chat: { id: string }): number =>
  d.routes().filter((one) => one === routeOf(chat)).length;

const toasts = (M: { S: { toasts: readonly { msg: string }[] } }): string[] =>
  M.S.toasts.map((toast) => toast.msg);

/** One event of a chat's own topic, as the daemon publishes it. */
const chatEvent = (chat: { id: string }, type: string, data: object): WireEvent => ({
  seq: 1,
  topic: `chat:${chat.id}`,
  type: type as WireEvent["type"],
  at: "2026-09-26T12:00:00.000Z",
  data: { cardId: "", chatId: chat.id, ...data },
});
const output = (chat: { id: string }, text: string) =>
  chatEvent(chat, EventTypeSessionOutput, { kind: "message", text });
const state = (chat: { id: string }, value: string, reason = "") =>
  chatEvent(chat, EventTypeSessionStateChanged, { sessionId: "s1", state: value, reason });
const toolCall = (chat: { id: string }, title: string, status: string) =>
  chatEvent(chat, EventTypeSessionToolCall, {
    kind: "tool_call",
    toolCall: { id: "call-9", title, toolKind: "other", status },
  });

describe("reading a chat's history when it opens", () => {
  it("puts the daemon's messages in the chat oldest first, in the words a card's chat is drawn with", async () => {
    const { M, chat, d } = await setup();
    expect(chat().msgs).toEqual([]);
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(3));
    expect(d.routes()).toContain(routeOf(CHAT));
    expect(chat().msgs.map((one) => one.id)).toEqual(["m1", "m2", "m3"]);
    expect(chat().msgs[0]).toEqual({ id: "m1", k: "user", text: "What is blocked?" });
    expect(chat().msgs[1]).toMatchObject({ k: "tool", action: "List the cards", st: "ok" });
    expect(chat().msgs[2]).toMatchObject({ k: "agent", text: "Nothing is blocked right now." });
    expect(chat().history).toBeUndefined();
  });

  it("says it is loading until the daemon answers, and shows nothing older beside the new read", async () => {
    const { M, chat, d } = await setup();
    const release = d.holdNext(routeOf(CHAT));
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().history).toBe("loading"));
    expect(chat().msgs).toEqual([]);
    release();
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(3));
    expect(chat().history).toBeUndefined();
  });

  it("leaves a chat with no messages empty, and not loading, for the invitation to say something", async () => {
    const { M, chat } = await setup();
    M.openChat("api", FRESH.id);
    await vi.waitFor(() => expect(chat(FRESH).history).toBeUndefined());
    await vi.waitFor(() => expect(daemon?.routes()).toContain(routeOf(FRESH)));
    expect(chat(FRESH).msgs).toEqual([]);
  });

  it("says the daemon's own sentence when the history cannot be read, and tries again on request", async () => {
    const { M, chat, d } = await setup();
    d.refuseNext(routeOf(CHAT), 500, "internal", "The daemon could not read this chat.");
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().history).toBe("failed"));
    expect(chat().historyError).toBe("The daemon could not read this chat.");
    expect(chat().msgs).toEqual([]);
    M.retryChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(3));
    expect(chat().history).toBeUndefined();
    expect(chat().historyError).toBeUndefined();
  });

  it("says so when the daemon is away, and reads it again once it is back", async () => {
    const { M, chat, d } = await setup();
    d.stop();
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().history).toBe("failed"));
    expect(chat().historyError).toBeTruthy();
    d.start();
    M.retryChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(3));
  });

  it("reads the chat again from the start each time it is opened, so an older read never sits beside a newer one", async () => {
    const { M, chat, d } = await setup();
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(3));
    M.openChat("api", OTHER.id);
    await vi.waitFor(() => expect(messageCalls(d, OTHER)).toBe(1));
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(messageCalls(d, CHAT)).toBe(2));
    await vi.waitFor(() => expect(chat().msgs.map((one) => one.id)).toEqual(["m1", "m2", "m3"]));
  });

  it("drops a read that a newer read of the same chat overtook, whichever answer comes last", async () => {
    const { M, ctx, d, chat } = await setup();
    // A daemon whose answers this test hands out by hand: the first read is asked for first and
    // answered last.
    const answers: ((page: unknown) => void)[] = [];
    const api = {
      chatMessages: () => new Promise((resolve) => answers.push(resolve)),
    } as unknown as typeof d.data.api;
    M.go("project", "api", "board");
    const first = readOpenChat(ctx, api, chat());
    const second = readOpenChat(ctx, api, chat());
    const page = (rows: typeof HISTORY) => ({
      items: rows.map((row) => row.message).sort((a, b) => b.seq - a.seq),
      nextCursor: "",
      serverTime: "2026-09-26T12:00:00.000Z",
    });
    answers[1]?.(page(HISTORY.slice(0, 1)));
    await second;
    expect(chat().msgs.map((one) => one.id)).toEqual(["m1"]);
    expect(chat().history).toBeUndefined();
    answers[0]?.(page(HISTORY));
    await first;
    expect(chat().msgs.map((one) => one.id)).toEqual(["m1"]);
  });

  it("does not read a chat that is not open, or that is open behind another view", async () => {
    const { M, d } = await setup();
    expect(d.routes().filter((one) => one.includes("/messages"))).toEqual([]);
    M.openChat("api", CHAT.id);
    M.go("project", "api", "board");
    await vi.waitFor(() => expect(lastTopics(d)).not.toContain(`chat:${CHAT.id}`));
  });

  it("does nothing while the chats are still the mock's", async () => {
    const d = createFakeDaemon({
      projects: PROTOTYPE_PROJECTS,
      chats: [CHAT],
      chatMessages: HISTORY,
    });
    daemon = d;
    const M = createTestMarshal({ data: d.data, sections: { ...ON_DAEMON, S17: "mock" } });
    await d.connect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    M.go("project", "api", "chat");
    const first = M.chatsOf("api")[0];
    if (first) M.openChat("api", first.id);
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(d.routes().filter((one) => one.includes("/chats/"))).toEqual([]);
  });
});

describe("following the open chat's own topic", () => {
  it("subscribes to the chat when it opens, moves to the next chat, and lets go when the view is left", async () => {
    const { M, d } = await setup();
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(lastTopics(d)).toContain(`chat:${CHAT.id}`));
    M.openChat("api", OTHER.id);
    await vi.waitFor(() => expect(lastTopics(d)).toContain(`chat:${OTHER.id}`));
    expect(lastTopics(d)).not.toContain(`chat:${CHAT.id}`);
    M.go("project", "api", "board");
    await vi.waitFor(() => expect(lastTopics(d)).not.toContain(`chat:${OTHER.id}`));
    M.go("project", "api", "chat");
    await vi.waitFor(() => expect(lastTopics(d)).toContain(`chat:${OTHER.id}`));
  });

  it("closes the topic when the open chat is closed", async () => {
    const { M, d } = await setup();
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(lastTopics(d)).toContain(`chat:${CHAT.id}`));
    M.openChat("api", null);
    await vi.waitFor(() => expect(lastTopics(d)).not.toContain(`chat:${CHAT.id}`));
  });

  it("asks the stream to subscribe and unsubscribe, one topic at a time", async () => {
    const { ctx, d } = await setup();
    const topics: string[] = [];
    const stream = {
      subscribe: (list: readonly string[]) => topics.push(...list.map((one) => `+${one}`)),
      unsubscribe: (list: readonly string[]) => topics.push(...list.map((one) => `-${one}`)),
    };
    const dispose = createRoot((stop) => {
      followOpenChat(ctx, d.data.api, stream);
      return stop;
    });
    ctx.S.chatOpen.api = CHAT.id;
    await vi.waitFor(() => expect(topics).toEqual([`+chat:${CHAT.id}`]));
    ctx.S.chatOpen.api = OTHER.id;
    await vi.waitFor(() =>
      expect(topics).toEqual([`+chat:${CHAT.id}`, `-chat:${CHAT.id}`, `+chat:${OTHER.id}`]),
    );
    dispose();
  });

  it("reads the open chat again when the connection comes back, since what was said meanwhile is not replayed", async () => {
    const { M, d, chat } = await setup();
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(3));
    d.chatMessages.push(
      chatMessageRow(CHAT.id, {
        id: "m4",
        kind: "agent",
        seq: 4,
        text: "Said while you were away.",
      }),
    );
    d.sockets.last().push({
      type: "resync",
      epoch: "01M3C0ZZZZ000000000000000B",
      reason: "epoch-changed",
      seq: 0,
    });
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(4));
    expect(chat().msgs.at(-1)).toMatchObject({ text: "Said while you were away." });
  });
});

describe("a chat's own live events", () => {
  it("joins the words of one answer into the message that is still streaming", async () => {
    const { M, ctx, chat } = await setup();
    M.openChat("api", FRESH.id);
    await vi.waitFor(() => expect(chat(FRESH).history).toBeUndefined());
    applyChatSessionEvent(ctx, output(FRESH, "Hello"));
    applyChatSessionEvent(ctx, output(FRESH, " world"));
    expect(chat(FRESH).msgs).toEqual([
      { id: expect.stringContaining("s"), k: "agent", text: "Hello world", streaming: true },
    ]);
  });

  it("arrives on the chat's topic, and is drawn in the chat and nowhere else", async () => {
    const { M, d, chat } = await setup();
    M.openChat("api", FRESH.id);
    await vi.waitFor(() => expect(lastTopics(d)).toContain(`chat:${FRESH.id}`));
    d.emit(`chat:${FRESH.id}`, EventTypeSessionOutput, {
      cardId: "",
      chatId: FRESH.id,
      kind: "message",
      text: "From the stream",
    });
    await vi.waitFor(() => expect(chat(FRESH).msgs).toHaveLength(1));
    expect(chat(FRESH).msgs[0]).toMatchObject({ k: "agent", text: "From the stream" });
    expect(chat(OTHER).msgs).toEqual([]);
    expect(M.S.cards.every((card) => card.session !== "working" || card.state !== "backlog")).toBe(
      true,
    );
  });

  it("shows a tool call as one line and keeps it up to date by its id, in the wire's own words", async () => {
    const { M, ctx, chat } = await setup();
    M.openChat("api", FRESH.id);
    await vi.waitFor(() => expect(chat(FRESH).history).toBeUndefined());
    applyChatSessionEvent(ctx, output(FRESH, "Let me look."));
    applyChatSessionEvent(ctx, toolCall(FRESH, "Read README.md", "in_progress"));
    expect(chat(FRESH).msgs.map((one) => one.k)).toEqual(["agent", "tool"]);
    expect(chat(FRESH).msgs[0]).toMatchObject({ streaming: false });
    expect(chat(FRESH).msgs[1]).toMatchObject({ action: "Read README.md", st: "running" });
    applyChatSessionEvent(ctx, toolCall(FRESH, "", "completed"));
    expect(chat(FRESH).msgs).toHaveLength(2);
    expect(chat(FRESH).msgs[1]).toMatchObject({ action: "Read README.md", st: "ok" });
  });

  it("ends the answer when the session goes back to waiting, so the next one is its own message", async () => {
    const { M, ctx, chat } = await setup();
    M.openChat("api", FRESH.id);
    await vi.waitFor(() => expect(chat(FRESH).history).toBeUndefined());
    applyChatSessionEvent(ctx, state(FRESH, "working"));
    applyChatSessionEvent(ctx, output(FRESH, "One"));
    expect(chat(FRESH).msgs[0]).toMatchObject({ streaming: true });
    applyChatSessionEvent(ctx, state(FRESH, "awake"));
    expect(chat(FRESH).msgs[0]).toMatchObject({ text: "One", streaming: false });
    applyChatSessionEvent(ctx, output(FRESH, "Two"));
    expect(chat(FRESH).msgs.map((one) => (one.k === "agent" ? one.text : one.k))).toEqual([
      "One",
      "Two",
    ]);
  });

  it("says why a session stopped, in the daemon's own sentence, as a note in the thread", async () => {
    const { M, ctx, chat } = await setup();
    M.openChat("api", FRESH.id);
    await vi.waitFor(() => expect(chat(FRESH).history).toBeUndefined());
    const why =
      "Marshal could not pick this chat's conversation back up. Start a new chat to keep going.";
    applyChatSessionEvent(ctx, state(FRESH, "stopped", why));
    expect(chat(FRESH).msgs).toEqual([{ id: expect.any(String), k: "system", text: why }]);
    // A session that stops with nothing to say leaves the thread alone.
    applyChatSessionEvent(ctx, state(FRESH, "stopped"));
    expect(chat(FRESH).msgs).toHaveLength(1);
  });

  it("waits for the history, and is drawn after it, in order and once", async () => {
    const { M, chat, d } = await setup();
    const release = d.holdNext(routeOf(CHAT));
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(lastTopics(d)).toContain(`chat:${CHAT.id}`));
    d.emit(`chat:${CHAT.id}`, EventTypeSessionOutput, {
      cardId: "",
      chatId: CHAT.id,
      kind: "message",
      text: "Live one",
    });
    d.emit(`chat:${CHAT.id}`, EventTypeSessionOutput, {
      cardId: "",
      chatId: CHAT.id,
      kind: "message",
      text: " and more",
    });
    // The chunks are in hand, and the thread is empty until the history is in.
    await vi.waitFor(() => expect(d.sockets.last().readyState).toBe(1));
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(chat().msgs).toEqual([]);
    release();
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(4));
    expect(
      chat()
        .msgs.map((one) => one.id)
        .slice(0, 3),
    ).toEqual(["m1", "m2", "m3"]);
    expect(chat().msgs[3]).toMatchObject({
      k: "agent",
      text: "Live one and more",
      streaming: true,
    });
  });

  it("leaves a card's events, an event that names no chat, and a chat the store does not hold alone", async () => {
    const { M, ctx, chat } = await setup();
    M.openChat("api", FRESH.id);
    await vi.waitFor(() => expect(chat(FRESH).history).toBeUndefined());
    applyChatSessionEvent(ctx, {
      seq: 1,
      topic: "card:01M3CARD",
      type: EventTypeSessionOutput,
      at: "2026-09-26T12:00:00.000Z",
      data: { cardId: "01M3CARD", kind: "message", text: "A card's words" },
    });
    applyChatSessionEvent(ctx, {
      ...output(FRESH, "nobody"),
      data: { kind: "message", text: "nobody" },
    });
    applyChatSessionEvent(ctx, output({ id: "01M3CHATNOPE" }, "not here"));
    applyChatSessionEvent(ctx, chatEvent(FRESH, "session.plan", { text: "ignored" }));
    expect(chat(FRESH).msgs).toEqual([]);
  });
});

describe("sending the person's own message", () => {
  it("shows it at once, asks the daemon for it, and the agent's answer streams in and is kept", async () => {
    const { M, chat, d } = await setup();
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(3));
    const release = d.holdNext(sendRoute(CHAT));
    M.chatSend("api", CHAT.id, "  Anything else?  ");
    // The person's words are in the thread before the daemon has answered the request.
    await vi.waitFor(() =>
      expect(chat().msgs.at(-1)).toMatchObject({ k: "user", text: "Anything else?" }),
    );
    release();
    await vi.waitFor(() => expect(d.bodies(sendRoute(CHAT))).toEqual([{ text: "Anything else?" }]));
    // The stub agent's answer arrives on the chat's topic and is drawn as it comes.
    await vi.waitFor(() =>
      expect(chat().msgs.at(-1)).toMatchObject({
        k: "agent",
        text: "The daemon answered: Anything else?",
      }),
    );
    // It ends when the session is back to waiting.
    await vi.waitFor(() => expect(chat().msgs.at(-1)).toMatchObject({ streaming: false }));
    expect(chat().msgs).toHaveLength(5);
    expect(toasts(M)).toEqual([]);
    // A reload of the page reads the same conversation from the daemon.
    const reloaded = createTestMarshal({ data: d.data, sections: ON_DAEMON });
    await vi.waitFor(() => expect(reloaded.S.ready).toBe(true));
    reloaded.go("project", "api", "chat");
    reloaded.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(reloaded.chatById("api", CHAT.id)?.msgs).toHaveLength(5));
    expect(reloaded.chatById("api", CHAT.id)?.msgs.slice(-2)).toMatchObject([
      { k: "user", text: "Anything else?" },
      { k: "agent", text: "The daemon answered: Anything else?" },
    ]);
  });

  it("names a chat that is still New chat after its first message, from the daemon's own chat.updated", async () => {
    const { M, chat } = await setup();
    M.openChat("api", FRESH.id);
    await vi.waitFor(() => expect(chat(FRESH).history).toBeUndefined());
    expect(chat(FRESH).title).toBe("New chat");
    M.chatSend("api", FRESH.id, "Please look at the flaky tests now thanks!");
    await vi.waitFor(() => expect(chat(FRESH).title).toBe("Please look at the flaky tests"));
    // A chat that already has a name keeps it.
    M.chatSend("api", CHAT.id, "Another question please");
    await vi.waitFor(() => expect(chat(CHAT).last).toBeGreaterThan(0));
    expect(chat(CHAT).title).toBe("Plan the export");
  });

  it.each([
    [
      "the chat is archived",
      "chat archived",
      422,
      "refused",
      "This chat is archived. Restore it to keep talking.",
    ],
    [
      "the conversation cannot be picked back up",
      "cannot resume",
      422,
      "refused",
      "Marshal could not pick this chat's conversation back up. Start a new chat to keep going.",
    ],
    [
      "the agent will not start",
      "agent could not start",
      503,
      "unavailable",
      "Marshal could not start the agent for this chat. Check that the agent is installed and that you are signed in to it, then try again.",
    ],
    [
      "the queue is full",
      "queue full",
      409,
      "conflict",
      "There are already too many messages waiting for this agent. Wait for it to catch up.",
    ],
  ])(
    "takes the words back out and shows the daemon's sentence when %s",
    async (_why, _name, status, code, sentence) => {
      const { M, chat, d } = await setup();
      M.openChat("api", CHAT.id);
      await vi.waitFor(() => expect(chat().msgs).toHaveLength(3));
      d.refuseNext(sendRoute(CHAT), status, code, sentence);
      M.chatSend("api", CHAT.id, "Hello");
      await vi.waitFor(() => expect(toasts(M)).toEqual([sentence]));
      expect(chat().msgs.map((one) => one.id)).toEqual(["m1", "m2", "m3"]);
    },
  );

  it("is refused by the daemon for a chat that is archived, and the archived chat says so in its own words", async () => {
    const { M, chat } = await setup();
    M.openChat("api", ARCHIVED.id);
    await vi.waitFor(() => expect(chat(ARCHIVED).history).toBeUndefined());
    M.chatSend("api", ARCHIVED.id, "Still there?");
    await vi.waitFor(() =>
      expect(toasts(M)).toEqual(["This chat is archived. Restore it to keep talking."]),
    );
    expect(chat(ARCHIVED).msgs).toEqual([]);
  });

  it("shows the daemon's sentence for an empty message, and sends nothing for blank text", async () => {
    const { M, ctx, d, chat } = await setup();
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(3));
    expect(await sendToChat(ctx, "api", CHAT.id, "   ")).toBe(false);
    expect(d.routes().filter((one) => one === sendRoute(CHAT))).toEqual([]);
  });

  it("waits for the history that is being read, so the message never lands under an older read", async () => {
    const { M, chat, d } = await setup();
    const release = d.holdNext(routeOf(CHAT));
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().history).toBe("loading"));
    M.chatSend("api", CHAT.id, "First words");
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(d.routes().filter((one) => one === sendRoute(CHAT))).toEqual([]);
    expect(chat().msgs).toEqual([]);
    release();
    await vi.waitFor(() => expect(d.bodies(sendRoute(CHAT))).toEqual([{ text: "First words" }]));
    expect(
      chat()
        .msgs.map((one) => one.id)
        .slice(0, 3),
    ).toEqual(["m1", "m2", "m3"]);
    expect(
      chat()
        .msgs.filter((one) => one.k === "user")
        .map((one) => (one.k === "user" ? one.text : "")),
    ).toEqual(["What is blocked?", "First words"]);
  });

  it("runs none of the mock's scripted replies for a chat the daemon keeps", async () => {
    const { M, chat } = await setup();
    const cards = M.S.cards.length;
    M.openChat("api", CHAT.id);
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(3));
    M.chatSend("api", CHAT.id, "Make cards for the billing export");
    await vi.waitFor(() => expect(chat().msgs).toHaveLength(5));
    // The mock would have answered after half a second by making three cards and saying so.
    await new Promise((resolve) => setTimeout(resolve, 700));
    expect(M.S.cards).toHaveLength(cards);
    expect(chat().msgs).toHaveLength(5);
    expect(chat().msgs.some((one) => one.k === "card" || one.k === "links")).toBe(false);
  });

  it("says Marshal is not connected when there is no daemon, and shows nothing", async () => {
    const M = createTestMarshal({ sections: ON_DAEMON });
    const ctx = contextOf(M);
    ctx.S.chats.api = [
      {
        id: CHAT.id,
        pid: "api",
        title: "Plan",
        target: "Orchestrator",
        msgs: [],
        last: 0,
        archived: false,
      },
    ];
    expect(await sendToChat(ctx, "api", CHAT.id, "Hello")).toBe(false);
    expect(toasts(M)).toEqual(["Marshal is not connected to its daemon."]);
    expect(ctx.S.chats.api[0]?.msgs).toEqual([]);
  });

  it("does nothing for a chat the store does not have", async () => {
    const { ctx, d } = await setup();
    expect(await sendToChat(ctx, "api", "01M3CHATNOPE", "Hello")).toBe(false);
    expect(d.routes().filter((one) => one.endsWith("/messages"))).toEqual([]);
  });
});

describe("trying a read again", () => {
  it("does nothing for a chat the store does not have, and says so when there is no daemon", async () => {
    const { ctx, d } = await setup();
    retryChat(ctx, "api", "01M3CHATNOPE");
    expect(d.routes().filter((one) => one.includes("/messages"))).toEqual([]);
    const M = createTestMarshal({ sections: ON_DAEMON });
    const offline = contextOf(M);
    offline.S.chats.api = [
      {
        id: CHAT.id,
        pid: "api",
        title: "Plan",
        target: "Orchestrator",
        msgs: [],
        last: 0,
        archived: false,
      },
    ];
    retryChat(offline, "api", CHAT.id);
    expect(toasts(M)).toEqual(["Marshal is not connected to its daemon."]);
  });
});
