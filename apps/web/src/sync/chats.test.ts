import type { Chat as WireChat } from "@marshal/protocol";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toMillis } from "~/data/mappers/time";
import { sectionStatus } from "~/data/sections";
import { golden } from "~/data/testing/golden";
import { wireChat } from "~/testing/fake-chats";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import {
  applyChat,
  applyChatRemoved,
  applyChatSnapshot,
  chatsSyncer,
  storedChat,
  targetOf,
} from "./chats";

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});
const open = (options?: Parameters<typeof createFakeDaemon>[0]): FakeDaemon => {
  daemon = createFakeDaemon(options);
  return daemon;
};

/** What section S17 says once the project chats are switched to the daemon. */
const onDaemon = (d: FakeDaemon) => ({
  data: d.data,
  sections: { ...sectionStatus, S17: "daemon" as const },
});

/** A store that follows the daemon, waited for until its first snapshots are in. */
async function synced(d: FakeDaemon) {
  const M = createTestMarshal(onDaemon(d));
  await d.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  return M;
}

/** The topics the client last told the daemon it wants, which is what a hello carries. */
const lastTopics = (d: FakeDaemon): string[] =>
  (d.sockets.last().hellos().at(-1)?.subscribe as string[] | undefined) ?? [];

const apiChat = (fields: Partial<WireChat> = {}): WireChat =>
  wireChat({ id: "01M3C107JB041061050R3GG281", projectId: "api", ...fields });

/** Tells the client that events were missed, as the daemon does after a break, so it loads again. */
function resync(d: FakeDaemon, epoch: string): void {
  d.sockets.last().push({ type: "resync", epoch, reason: "epoch-changed", seq: 0 });
}

/** One event as the stream delivers it, for the tests that call the handler directly. */
function eventOf(type: string, data: unknown) {
  return { seq: 1, topic: "project:api", type, at: "2026-09-30T12:00:00.000Z", data } as never;
}

describe("the chats section", () => {
  it("is section S17, no topic of its own, one per project, and it reads the chat events", () => {
    expect(chatsSyncer.section).toBe("S17");
    expect(chatsSyncer.topics).toEqual([]);
    expect(chatsSyncer.projectTopics?.("api")).toEqual(["project:api"]);
    expect(chatsSyncer.onEvent).toBeTypeOf("function");
  });

  it("draws no chats of its own while the section is still on the mock", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS, chats: [apiChat()] });
    const M = createTestMarshal({ data: d.data, sections: { ...sectionStatus, S17: "mock" } });
    await d.connect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    expect(d.routes()).not.toContain("GET /v1/projects/api/chats");
    expect(M.S.chats.api?.some((chat) => chat.id === "01M3C107JB041061050R3GG281")).toBeFalsy();
  });

  it("loads a project's live and archived chats, because the view keeps both halves", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS.slice(0, 1), chats: [apiChat()] });
    const M = await synced(d);
    expect(d.routes()).toContain("GET /v1/projects/api/chats");
    expect(d.routes()).toContain("GET /v1/projects/api/chats?archived=true");
    expect(M.S.chats.api?.map((chat) => chat.id)).toEqual(["01M3C107JB041061050R3GG281"]);
  });

  it("reads every project's chats, one call each, since no call lists them across projects", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS.slice(0, 2) });
    await synced(d);
    expect(d.routes()).toContain("GET /v1/projects/api/chats");
    expect(d.routes()).toContain("GET /v1/projects/web/chats");
  });

  it("subscribes to each project's topic, so a chat made elsewhere arrives", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS.slice(0, 1) });
    const M = await synced(d);
    await vi.waitFor(() => expect(lastTopics(d)).toContain("project:api"));
    d.emit("project:api", "chat.created", { chat: apiChat({ title: "Made elsewhere" }) });
    await vi.waitFor(() => expect(M.S.chats.api?.[0]?.title).toBe("Made elsewhere"));
  });

  it("subscribes to a project that appears, and unsubscribes from one that is removed", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS.slice(0, 2) });
    await synced(d);
    await vi.waitFor(() => expect(lastTopics(d)).toContain("project:web"));
    d.projects.pop();
    resync(d, "01M3C0ZZZZ000000000000000C");
    await vi.waitFor(() => expect(lastTopics(d)).not.toContain("project:web"));
  });

  it("takes a chat the daemon no longer lists out of the store", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS.slice(0, 1), chats: [apiChat()] });
    const M = await synced(d);
    expect(M.S.chats.api).toHaveLength(1);
    d.chats.length = 0;
    resync(d, "01M3C0ZZZZ000000000000000D");
    await vi.waitFor(() => expect(M.S.chats.api).toHaveLength(0));
  });
});

describe("the chat mapper", () => {
  it("reads the golden chat's own fields into the shape the view draws", () => {
    const wire = golden<WireChat>("chat");
    const chat = storedChat(wire);
    expect(chat).toEqual({
      id: wire.id,
      pid: wire.projectId,
      title: wire.title,
      target: "Worker",
      msgs: [],
      last: toMillis(wire.lastActiveAt),
      archived: false,
      fresh: false,
    });
  });

  it("reads an archived chat as an archived row", () => {
    const wire = { ...golden<WireChat>("chat"), archivedAt: "2026-09-30T11:30:00.000Z" };
    expect(storedChat(wire).archived).toBe(true);
  });

  it("turns the Orchestrator into the name the list shows, whatever its id says", () => {
    expect(targetOf({ kind: "orchestrator", id: "" })).toBe("Orchestrator");
    expect(targetOf({ kind: "orchestrator", id: "ignored" })).toBe("Orchestrator");
  });

  it("keeps a role's own name as the target", () => {
    expect(targetOf({ kind: "role", id: "Tester" })).toBe("Tester");
  });

  it("turns a card's opaque id into the key the store knows the card by", () => {
    const cards = [{ id: "web#118", daemonId: "01M3C107JB041061050R3GG28A" }] as never;
    expect(targetOf({ kind: "card", id: "01M3C107JB041061050R3GG28A" }, cards)).toBe("web#118");
  });

  it("shows the opaque id when the card is not in the store yet", () => {
    expect(targetOf({ kind: "card", id: "01M3C107JB041061050R3GG28A" }, [])).toBe(
      "01M3C107JB041061050R3GG28A",
    );
  });
});

describe("applying a snapshot", () => {
  it("adds the daemon's chats to the projects it knows, and leaves other projects alone", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.chats = {};
    applyChatSnapshot(ctx, [
      apiChat({ title: "One" }),
      apiChat({ id: "01M3C107JB041061050R3GG282", title: "Two" }),
    ]);
    expect(ctx.S.chats.api?.map((chat) => chat.id)).toEqual([
      "01M3C107JB041061050R3GG281",
      "01M3C107JB041061050R3GG282",
    ]);
    expect(ctx.S.chats.api?.map((chat) => chat.title)).toEqual(["One", "Two"]);
    expect(ctx.S.chats.web ?? []).toEqual([]);
  });

  it("keeps a chat that is already there, and keeps its messages", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.chats = {};
    applyChatSnapshot(ctx, [apiChat()]);
    const chat = ctx.S.chats.api?.[0];
    if (!chat) throw new Error("no chat");
    chat.msgs = [{ id: "m1", k: "user", text: "Keep me" }];
    applyChatSnapshot(ctx, [apiChat({ title: "Renamed elsewhere" })]);
    expect(ctx.S.chats.api).toHaveLength(1);
    expect(ctx.S.chats.api?.[0]).toBe(chat);
    expect(chat.title).toBe("Renamed elsewhere");
    expect(chat.msgs).toHaveLength(1);
  });

  it("changes nothing when the same snapshot is applied twice", () => {
    const ctx = contextOf(createTestMarshal());
    applyChatSnapshot(ctx, [apiChat()]);
    const first = ctx.S.chats.api;
    applyChatSnapshot(ctx, [apiChat()]);
    expect(ctx.S.chats.api).toBe(first);
  });
});

describe("applying one chat", () => {
  it("adds a chat the store did not have", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.chats = {};
    applyChat(ctx, apiChat());
    expect(ctx.S.chats.api?.map((chat) => chat.id)).toEqual(["01M3C107JB041061050R3GG281"]);
  });

  it("updates a chat in place, so a chat that did not change is not redrawn", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.chats = {};
    applyChat(ctx, apiChat());
    const chat = ctx.S.chats.api?.[0];
    applyChat(ctx, apiChat({ title: "Changed" }));
    expect(ctx.S.chats.api?.[0]).toBe(chat);
    expect(chat?.title).toBe("Changed");
  });

  it("takes a removed chat out, and closes it when it was open", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.chats = {};
    applyChat(ctx, apiChat());
    ctx.S.chatOpen.api = "01M3C107JB041061050R3GG281";
    applyChatRemoved(ctx, "01M3C107JB041061050R3GG281", "api");
    expect(ctx.S.chats.api).toEqual([]);
    expect(ctx.S.chatOpen.api).toBeNull();
  });

  it("applies a chat.created event twice without adding the chat twice", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.chats = {};
    const event = eventOf("chat.created", { chat: apiChat() });
    chatsSyncer.onEvent?.(ctx, event);
    chatsSyncer.onEvent?.(ctx, event);
    expect(ctx.S.chats.api).toHaveLength(1);
  });

  it("takes a chat out on chat.deleted, which carries its id and its project", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.chats = {};
    applyChat(ctx, apiChat());
    chatsSyncer.onEvent?.(
      ctx,
      eventOf("chat.deleted", { chatId: "01M3C107JB041061050R3GG281", projectId: "api" }),
    );
    expect(ctx.S.chats.api).toEqual([]);
  });

  it("ignores an event that is not a chat event", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.chats = {};
    chatsSyncer.onEvent?.(ctx, eventOf("card.updated", { card: { id: "api#1" } }));
    expect(ctx.S.chats.api ?? []).toEqual([]);
  });
});
