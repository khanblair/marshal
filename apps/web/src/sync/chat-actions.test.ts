import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import { wireCard } from "~/testing/fake-cards";
import { wireChat } from "~/testing/fake-chats";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { archiveChat, deleteChat, newChat, renameChat } from "./chat-actions";
import { wireTargetOf } from "./chats";

// Section S17, the chat management (docs/backend-checklist.md B2.10): a chat is made, renamed,
// archived, restored, and deleted through the daemon, and every refusal is the daemon's own sentence.
const ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const, S17: "daemon" as const };

const CHAT = wireChat({
  id: "01M3CHAT00000000000000B001",
  projectId: "api",
  title: "Plan the export",
});
const CARD = wireCard({
  projectId: "api",
  number: 41,
  title: "Fix token refresh",
  state: "working",
});

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

async function setup() {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, chats: [CHAT], cards: [CARD] });
  daemon = d;
  const M = createTestMarshal({ data: d.data, sections: ON_DAEMON });
  await d.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  let last = -1;
  await vi.waitFor(() => {
    const loads = d.routes().filter((one) => one.endsWith("/chats")).length;
    const steady = loads > 0 && loads === last;
    last = loads;
    expect(steady).toBe(true);
  });
  return { M, ctx: contextOf(M), d };
}

const toasts = (M: { S: { toasts: readonly { msg: string }[] } }): string[] =>
  M.S.toasts.map((toast) => toast.msg);

describe("the wire target of a store target", () => {
  it("reads the Orchestrator, a role, and a card by its key, which is what a chat's target is called", () => {
    const cards = contextOf(createTestMarshal({ data: null, sections: ON_DAEMON })).S.cards;
    expect(wireTargetOf("Orchestrator")).toEqual({ kind: "orchestrator", id: "" });
    expect(wireTargetOf("Tester")).toEqual({ kind: "role", id: "Tester" });
    // A card is named by its opaque id on the wire, never by its key; one the store does not hold is a role.
    const card = cards.find((one) => one.daemonId !== undefined);
    if (card) expect(wireTargetOf(card.id, cards)).toEqual({ kind: "card", id: card.daemonId });
    expect(wireTargetOf("api#9999", cards)).toEqual({ kind: "role", id: "api#9999" });
  });
});

describe("making a chat", () => {
  it("asks the daemon for a chat that talks to the Orchestrator, adds it, and opens it", async () => {
    const { M, d } = await setup();
    const made = await newChat(contextOf(M), "api");
    expect(d.bodies("POST /v1/projects/api/chats")).toEqual([
      { target: { kind: "orchestrator", id: "" } },
    ]);
    expect(made).toMatchObject({ pid: "api", title: "New chat", target: "Orchestrator", msgs: [] });
    expect(M.S.chatOpen.api).toBe(made?.id);
    expect(M.chatsOf("api").map((chat) => chat.id)).toContain(made?.id);
    // The chat.created that follows finds the chat as it is: nothing is drawn twice.
    await vi.waitFor(() => expect(d.sockets.last().readyState).toBe(1));
    d.emit("project:api", "chat.created", { chat: d.chats.at(-1) });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(M.chatsOf("api").filter((chat) => chat.id === made?.id)).toHaveLength(1);
  });

  it("names a role by its own name, and a card by its opaque id", async () => {
    const { M, d } = await setup();
    await newChat(contextOf(M), "api", "Tester");
    await newChat(contextOf(M), "api", "api#41");
    expect(d.bodies("POST /v1/projects/api/chats")).toEqual([
      { target: { kind: "role", id: "Tester" } },
      { target: { kind: "card", id: CARD.id } },
    ]);
    expect(M.chatsOf("api").map((chat) => chat.target)).toEqual(
      expect.arrayContaining(["Tester", "api#41"]),
    );
  });

  it("shows the daemon's sentence and adds nothing when it refuses", async () => {
    const { M, d } = await setup();
    d.refuseNext(
      "POST /v1/projects/api/chats",
      400,
      "invalid_argument",
      "Choose the role this chat talks to.",
    );
    expect(await newChat(contextOf(M), "api", "")).toBeNull();
    expect(toasts(M)).toEqual(["Choose the role this chat talks to."]);
    expect(M.chatsOf("api")).toHaveLength(1);
  });

  it("says Marshal is not connected when there is no daemon", async () => {
    const M = createTestMarshal({ sections: ON_DAEMON });
    expect(await newChat(contextOf(M), "api")).toBeNull();
    expect(toasts(M)).toEqual(["Marshal is not connected to its daemon."]);
  });
});

describe("renaming a chat", () => {
  it("renames it from the daemon's answer", async () => {
    const { M, ctx, d } = await setup();
    expect(await renameChat(ctx, "api", CHAT.id, "  Export plan  ")).toBe(true);
    expect(d.bodies(`PATCH /v1/chats/${CHAT.id}`)).toEqual([{ title: "  Export plan  " }]);
    expect(M.chatById("api", CHAT.id)?.title).toBe("Export plan");
  });

  it("keeps the old name and shows the daemon's sentence for a name that is only spaces", async () => {
    const { M, ctx } = await setup();
    expect(await renameChat(ctx, "api", CHAT.id, "   ")).toBe(false);
    expect(toasts(M)).toEqual(["Chat names can't be empty. The old name is kept."]);
    expect(M.chatById("api", CHAT.id)?.title).toBe("Plan the export");
  });

  it("does nothing for a chat the store does not have", async () => {
    const { ctx, d } = await setup();
    expect(await renameChat(ctx, "api", "01M3CHATNOPE", "x")).toBe(false);
    expect(d.routes().filter((one) => one.startsWith("PATCH"))).toEqual([]);
  });
});

describe("archiving and restoring a chat", () => {
  it("archives it, closes the pane when it was open, and offers to undo", async () => {
    const { M, ctx, d } = await setup();
    M.openChat("api", CHAT.id);
    await archiveChat(ctx, "api", CHAT.id, true);
    expect(d.routes()).toContain(`POST /v1/chats/${CHAT.id}/archive`);
    expect(M.chatById("api", CHAT.id)?.archived).toBe(true);
    expect(M.S.chatOpen.api).toBeNull();
    expect(toasts(M)).toEqual(["Chat archived"]);
    const undo = M.S.toasts[0]?.action;
    expect(undo?.label).toBe("Undo");
    undo?.run();
    await vi.waitFor(() => expect(M.chatById("api", CHAT.id)?.archived).toBe(false));
    expect(d.routes()).toContain(`POST /v1/chats/${CHAT.id}/restore`);
    expect(toasts(M)).toContain("Chat restored");
  });

  it("leaves another open chat open when a different one is archived", async () => {
    const { M, ctx } = await setup();
    M.openChat("api", "01M3CHATOTHER");
    await archiveChat(ctx, "api", CHAT.id, true);
    expect(M.S.chatOpen.api).toBe("01M3CHATOTHER");
  });

  it("shows the daemon's sentence and changes nothing when it refuses", async () => {
    const { M, ctx, d } = await setup();
    d.refuseNext(
      `POST /v1/chats/${CHAT.id}/archive`,
      500,
      "internal",
      "The daemon could not archive this chat.",
    );
    await archiveChat(ctx, "api", CHAT.id, true);
    expect(toasts(M)).toEqual(["The daemon could not archive this chat."]);
    expect(M.chatById("api", CHAT.id)?.archived).toBe(false);
  });

  it("does nothing for a chat the store does not have", async () => {
    const { ctx, d } = await setup();
    await archiveChat(ctx, "api", "01M3CHATNOPE", true);
    expect(d.routes().filter((one) => one.includes("/archive"))).toEqual([]);
  });
});

describe("deleting a chat", () => {
  it("asks first, with the words the design has, and deletes it when the dialog is confirmed", async () => {
    const { M, ctx, d } = await setup();
    deleteChat(ctx, "api", CHAT.id);
    expect(M.S.dialog).toMatchObject({
      title: "Delete chat",
      message:
        'This deletes "Plan the export" and its messages. Cards it created stay on the board.',
      action: "Delete chat",
      destructive: true,
    });
    expect(d.routes().filter((one) => one.startsWith("DELETE"))).toEqual([]);
    M.S.dialog?.run();
    await vi.waitFor(() => expect(M.chatById("api", CHAT.id)).toBeUndefined());
    expect(d.routes()).toContain(`DELETE /v1/chats/${CHAT.id}`);
    expect(toasts(M)).toEqual(["Chat deleted"]);
  });

  it("closes the pane when the deleted chat was open", async () => {
    const { M, ctx } = await setup();
    M.openChat("api", CHAT.id);
    deleteChat(ctx, "api", CHAT.id);
    M.S.dialog?.run();
    await vi.waitFor(() => expect(M.S.chatOpen.api).toBeNull());
  });

  it("keeps the chat and shows the daemon's sentence when it refuses", async () => {
    const { M, ctx, d } = await setup();
    d.refuseNext(
      `DELETE /v1/chats/${CHAT.id}`,
      500,
      "internal",
      "The daemon could not delete this chat.",
    );
    deleteChat(ctx, "api", CHAT.id);
    M.S.dialog?.run();
    await vi.waitFor(() => expect(toasts(M)).toEqual(["The daemon could not delete this chat."]));
    expect(M.chatById("api", CHAT.id)).toBeDefined();
  });

  it("asks nothing for a chat the store does not have", async () => {
    const { M, ctx } = await setup();
    deleteChat(ctx, "api", "01M3CHATNOPE");
    expect(M.S.dialog).toBeNull();
  });
});

describe("the chat actions the screens call", () => {
  it("are the daemon's once section S17 is switched, and the mock's own before", async () => {
    const { M, d } = await setup();
    await M.newChat("api", "Reviewer");
    expect(d.routes()).toContain("POST /v1/projects/api/chats");
    const mock = createTestMarshal({ sections: { ...ON_DAEMON, S17: "mock" } });
    const chat = await mock.newChat("api", "Reviewer");
    expect(chat).toMatchObject({ fresh: true, target: "Reviewer" });
  });
});
