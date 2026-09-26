// biome-ignore-all assist/source/organizeImports: the store `~/mock` builds has to be made in the hoisted block first, so it is the one that follows the fake daemon (S17 is the daemon's).
import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { contextOf } from "~/testing/test-store";
import { ChatsView } from "./ChatsView";

/*
 * The Chats view against a store whose project chats are the daemon's (section S17): a chat's history
 * is read when it opens, its loading, empty, and failed states are drawn in the thread, and what the
 * person sends and what the agent answers appear as they happen. The store and the daemon it follows
 * are built before any import runs, because the views read the one store `~/mock` hands out.
 */
const host = await vi.hoisted(async () => {
  window.location.hash = "#nosim";
  const { createFakeDaemon } = await import("~/testing/fake-daemon");
  const { chatMessageRow, wireChat } = await import("~/testing/fake-chats");
  const { PROTOTYPE_PROJECTS } = await import("~/testing/projects");
  const { createTestMarshal, DAEMON_CARDS } = await import("~/testing/test-store");
  const talk = wireChat({
    id: "01M3CHAT00000000000000C001",
    projectId: "api",
    title: "Plan the export",
  });
  const quiet = wireChat({
    id: "01M3CHAT00000000000000C002",
    projectId: "api",
    title: "Nothing said yet",
  });
  const history = [
    chatMessageRow(talk.id, { id: "h1", kind: "user", seq: 1, text: "What is blocked?" }),
    chatMessageRow(talk.id, {
      id: "h2",
      kind: "agent",
      seq: 2,
      text: "Nothing is blocked right now.",
    }),
  ];
  const daemon = createFakeDaemon({
    projects: PROTOTYPE_PROJECTS,
    chats: [talk, quiet],
    chatMessages: history,
  });
  window.M = createTestMarshal({
    data: daemon.data,
    sections: { ...DAEMON_CARDS, S17: "daemon" },
  });
  return { daemon, talk, quiet, history };
});

const { daemon, talk, quiet } = host;
const READ = (id: string) => `GET /v1/chats/${id}/messages?limit=50`;

/** Waits until the app has stopped asking for chat lists: the stream's first Resync makes it load once more. */
async function settled(): Promise<void> {
  let last = -1;
  await vi.waitFor(() => {
    const loads = daemon.routes().filter((route) => route.endsWith("/chats")).length;
    const steady = loads > 0 && loads === last;
    last = loads;
    expect(steady).toBe(true);
  });
}

beforeAll(async () => {
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  await settled();
});
afterAll(() => daemon.data.stop());

beforeEach(async () => {
  // Every test starts with the two chats and the two messages the daemon began with, and the Chats view
  // of the api project on screen with nothing chosen: the view opens the first chat itself, so the
  // chats have to be in the store before it is drawn.
  daemon.chats.splice(0, daemon.chats.length, structuredClone(talk), structuredClone(quiet));
  daemon.chatMessages.splice(
    0,
    daemon.chatMessages.length,
    ...host.history.map((row) => structuredClone(row)),
  );
  M.S.chats.api = [];
  M.S.chatOpen = {};
  M.S.toasts = [];
  M.S.dialog = null;
  M.setViewport(1440, 900);
  await contextOf(M).sync?.reload();
  await vi.waitFor(() => expect(M.chatsOf("api")).toHaveLength(2));
  M.go("project", "api", "chat");
});
afterEach(cleanup);

const thread = () => screen.getByRole("region", { name: /./ });
const composer = () => screen.getByRole("textbox", { name: "Message" });
const rows = () => within(screen.getByRole("complementary", { name: "Chats" }));

describe("the open chat's thread", () => {
  it("shows the messages the daemon kept, oldest first, once they are read", async () => {
    render(() => <ChatsView />);
    expect(await screen.findByText("Nothing is blocked right now.")).toBeInTheDocument();
    // The question is in the thread, beside the chip that suggests it.
    expect(screen.getAllByText("What is blocked?")).toHaveLength(2);
    expect(screen.queryByText("Loading messages")).not.toBeInTheDocument();
    expect(screen.queryByText(/The first message names this chat/)).not.toBeInTheDocument();
    expect(daemon.routes()).toContain(READ(talk.id));
  });

  it("draws a loading skeleton in the thread, and nothing of the invitation, until the history is in", async () => {
    await vi.waitFor(() => expect(M.chatById("api", talk.id)).toBeDefined());
    const release = daemon.holdNext(READ(talk.id));
    try {
      render(() => <ChatsView />);
      expect(await screen.findByText("Loading messages")).toBeInTheDocument();
      expect(screen.getByRole("status")).toContainElement(screen.getByText("Loading messages"));
      expect(screen.queryByText(/The first message names this chat/)).not.toBeInTheDocument();
      expect(thread().querySelector("[aria-busy='true']")).not.toBeNull();
    } finally {
      release();
    }
    expect(await screen.findByText("Nothing is blocked right now.")).toBeInTheDocument();
    expect(screen.queryByText("Loading messages")).not.toBeInTheDocument();
    expect(thread().querySelector("[aria-busy='true']")).toBeNull();
  });

  it("invites the person to say something in a chat that has no messages", async () => {
    render(() => <ChatsView />);
    fireEvent.click(await rows().findByRole("button", { name: /^Nothing said yet/ }));
    expect(
      await screen.findByText(
        /Say what you want in plain words. The first message names this chat./,
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("Loading messages")).not.toBeInTheDocument();
  });

  it("says why it is empty, in the daemon's own sentence, when the history cannot be read, and reads it again on Try again", async () => {
    await vi.waitFor(() => expect(M.chatById("api", talk.id)).toBeDefined());
    daemon.refuseNext(READ(talk.id), 500, "internal", "The daemon could not read this chat.");
    render(() => <ChatsView />);
    expect(await screen.findByText("The daemon could not read this chat.")).toBeInTheDocument();
    expect(screen.queryByText(/The first message names this chat/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Nothing is blocked right now.")).toBeInTheDocument();
    expect(screen.queryByText("The daemon could not read this chat.")).not.toBeInTheDocument();
  });
});

describe("talking in the chat", () => {
  it("sends what the person types, and the agent's answer streams in and stays", async () => {
    render(() => <ChatsView />);
    await screen.findByText("Nothing is blocked right now.");
    fireEvent.input(composer(), { target: { value: "And what is next?" } });
    fireEvent.click(screen.getByRole("button", { name: "Send message" }));
    expect(await screen.findByText("And what is next?")).toBeInTheDocument();
    expect(await screen.findByText("The daemon answered: And what is next?")).toBeInTheDocument();
    expect(daemon.bodies(`POST /v1/chats/${talk.id}/messages`)).toEqual([
      { text: "And what is next?" },
    ]);
    expect(composer()).toHaveValue("");
  });

  it("names a chat in the list after its first message, from what the daemon says", async () => {
    // The daemon names only a chat that is still "New chat", so this one is put back to that first.
    const wire = daemon.chats.find((chat) => chat.id === quiet.id);
    if (!wire) throw new Error("the daemon has no such chat");
    wire.title = "New chat";
    daemon.emit("project:api", "chat.updated", { chat: wire });
    await vi.waitFor(() => expect(M.chatById("api", quiet.id)?.title).toBe("New chat"));
    render(() => <ChatsView />);
    M.openChat("api", quiet.id);
    await screen.findByText(/The first message names this chat/);
    fireEvent.input(composer(), { target: { value: "Plan the billing export for June" } });
    fireEvent.click(screen.getByRole("button", { name: "Send message" }));
    await vi.waitFor(() =>
      expect(M.chatById("api", quiet.id)?.title).toBe("Plan the billing export for June"),
    );
    expect(await rows().findByText("Plan the billing export for June")).toBeInTheDocument();
  });

  it("shows the daemon's sentence and takes the words back when the daemon refuses", async () => {
    render(() => <ChatsView />);
    await screen.findByText("Nothing is blocked right now.");
    daemon.refuseNext(
      `POST /v1/chats/${talk.id}/messages`,
      422,
      "refused",
      "Marshal could not pick this chat's conversation back up. Start a new chat to keep going.",
    );
    fireEvent.input(composer(), { target: { value: "Are you there?" } });
    fireEvent.click(screen.getByRole("button", { name: "Send message" }));
    await vi.waitFor(() =>
      expect(M.S.toasts.map((toast) => toast.msg)).toEqual([
        "Marshal could not pick this chat's conversation back up. Start a new chat to keep going.",
      ]),
    );
    expect(screen.queryByText("Are you there?")).not.toBeInTheDocument();
  });

  it("makes a chat on the daemon from New chat, and opens it", async () => {
    render(() => <ChatsView />);
    await screen.findByText("Nothing is blocked right now.");
    M.set({ newChatOpen: true });
    fireEvent.click(await screen.findByRole("button", { name: "Start chat" }));
    await vi.waitFor(() => expect(daemon.routes()).toContain("POST /v1/projects/api/chats"));
    await vi.waitFor(() =>
      expect(M.chatById("api", M.S.chatOpen.api ?? "")?.title).toBe("New chat"),
    );
    expect(await screen.findByText(/The first message names this chat/)).toBeInTheDocument();
  });
});
