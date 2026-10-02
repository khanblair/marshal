// biome-ignore-all assist/source/organizeImports: the store `~/mock` builds has to be made in the hoisted block first, so it is the one that follows the fake daemon (S17 is the daemon's).
import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { contextOf } from "~/testing/test-store";
import { ChatsView } from "./ChatsView";

/*
 * The pinned Integrator chat against a store whose project chats are the daemon's (section S17): the
 * `system` and `permissionMode` the daemon sends reach the screen, the daemon's own refusal of a
 * rename, an archive, and a delete is shown as its sentence, and the warning about a chat that edits
 * the owner's folder follows the mode the daemon reports.
 */
const host = await vi.hoisted(async () => {
  window.location.hash = "#nosim";
  const { createFakeDaemon } = await import("~/testing/fake-daemon");
  const { wireChat, wireSystemChat } = await import("~/testing/fake-chats");
  const { PROTOTYPE_PROJECTS } = await import("~/testing/projects");
  const { createTestMarshal, DAEMON_CARDS } = await import("~/testing/test-store");
  const system = wireSystemChat("api");
  const worker = wireChat({
    id: "01M3CHAT00000000000000W001",
    projectId: "api",
    title: "Fix the flaky test",
    target: { kind: "role", id: "Worker" },
    permissionMode: "auto-edits",
    lastActiveAt: "2026-09-30T12:30:00.000Z",
  });
  const asking = wireChat({
    id: "01M3CHAT00000000000000W002",
    projectId: "api",
    title: "Tidy the readme",
    target: { kind: "role", id: "Worker" },
    permissionMode: "ask",
    lastActiveAt: "2026-09-30T12:20:00.000Z",
  });
  const planner = wireChat({
    id: "01M3CHAT00000000000000O001",
    projectId: "api",
    title: "Plan the export",
    target: { kind: "orchestrator", id: "" },
    permissionMode: "auto-edits",
    lastActiveAt: "2026-09-30T12:10:00.000Z",
  });
  const daemon = createFakeDaemon({
    projects: PROTOTYPE_PROJECTS,
    chats: [worker, asking, planner, system],
    chatMessages: [],
  });
  window.M = createTestMarshal({
    data: daemon.data,
    sections: { ...DAEMON_CARDS, S17: "daemon" },
  });
  return { daemon, system, worker, asking, planner };
});

const { daemon, system, worker, asking, planner } = host;
const WARNING = "This chat edits your folder directly.";

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
  daemon.chats.splice(
    0,
    daemon.chats.length,
    ...[worker, asking, planner, system].map((chat) => structuredClone(chat)),
  );
  M.S.chats.api = [];
  M.S.chatOpen = {};
  M.S.toasts = [];
  M.S.dialog = null;
  M.setViewport(1440, 900);
  await contextOf(M).sync?.reload();
  await vi.waitFor(() => expect(M.chatsOf("api")).toHaveLength(4));
  M.go("project", "api", "chat");
});
afterEach(cleanup);

const list = () => within(screen.getByRole("complementary", { name: "Chats" }));
const toasts = () => M.S.toasts.map((toast) => toast.msg);
const REFUSAL = (verb: string) =>
  `The Integrator chat is pinned to this project, so it can't be ${verb}.`;

describe("the Integrator chat the daemon keeps", () => {
  it("reaches the store as a system chat with the mode the daemon runs it in", () => {
    const stored = M.chatById("api", system.id);
    expect(stored).toMatchObject({
      title: "Integrator",
      target: "Integrator",
      system: "integrator",
      mode: "auto-edits",
    });
    expect(M.chatById("api", worker.id)?.system).toBeUndefined();
  });

  it("is the first row with its badge and no menu, ahead of chats that are more recent", () => {
    render(() => <ChatsView />);
    const first = list().getAllByRole("listitem")[0] as HTMLElement;
    expect(first).toHaveTextContent("Integrator");
    expect(
      within(first).getByText("Integrator", { selector: "span.rounded-xs" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "More actions for Integrator" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "More actions for Fix the flaky test" }),
    ).toBeInTheDocument();
  });

  it("stays first and a system chat when the daemon says it was touched", async () => {
    const wire = daemon.chats.find((chat) => chat.id === system.id);
    if (!wire) throw new Error("the daemon has no Integrator chat");
    wire.lastActiveAt = "2026-09-30T13:00:00.000Z";
    daemon.emit("project:api", "chat.updated", { chat: wire });
    await vi.waitFor(() => expect(M.chatById("api", system.id)?.last).toBeGreaterThan(0));
    expect(M.chatsOf("api")[0]?.id).toBe(system.id);
    expect(M.chatById("api", system.id)?.system).toBe("integrator");
  });
});

describe("what the daemon refuses to do to it", () => {
  it("answers a rename with its own sentence and leaves the name alone", async () => {
    await M.renameChat("api", system.id, "My merges");
    await vi.waitFor(() => expect(toasts()).toEqual([REFUSAL("renamed")]));
    expect(M.chatById("api", system.id)?.title).toBe("Integrator");
    expect(daemon.bodies(`PATCH /v1/chats/${system.id}`)).toEqual([{ title: "My merges" }]);
  });

  it("answers an archive with its own sentence and keeps the chat in the list", async () => {
    await M.archiveChat("api", system.id, true);
    await vi.waitFor(() => expect(toasts()).toEqual([REFUSAL("archived")]));
    expect(M.chatById("api", system.id)?.archived).toBe(false);
    expect(daemon.routes()).toContain(`POST /v1/chats/${system.id}/archive`);
  });

  it("answers a delete, once confirmed, with its own sentence and keeps the chat", async () => {
    M.deleteChat("api", system.id);
    expect(M.S.dialog).toMatchObject({ title: "Delete chat" });
    M.S.dialog?.run();
    await vi.waitFor(() => expect(toasts()).toEqual([REFUSAL("deleted")]));
    expect(M.chatById("api", system.id)).toBeDefined();
    expect(daemon.chats.some((chat) => chat.id === system.id)).toBe(true);
  });

  it("does not stop an ordinary chat from being renamed", async () => {
    await M.renameChat("api", worker.id, "Fix the flaky login test");
    await vi.waitFor(() =>
      expect(M.chatById("api", worker.id)?.title).toBe("Fix the flaky login test"),
    );
    expect(toasts()).toEqual([]);
  });
});

describe("the warning in the header, from the mode the daemon reports", () => {
  it("is shown for a Worker chat that edits without asking", () => {
    M.openChat("api", worker.id);
    render(() => <ChatsView />);
    expect(screen.getByRole("note")).toHaveTextContent(WARNING);
  });

  it("is not shown for a chat that asks first, an Orchestrator chat, or the Integrator chat", () => {
    render(() => <ChatsView />);
    for (const id of [asking.id, planner.id, system.id]) {
      M.openChat("api", id);
      expect(screen.queryByText(WARNING), id).not.toBeInTheDocument();
    }
  });

  it("goes away when the daemon's mode for the chat becomes plan-only", async () => {
    M.openChat("api", worker.id);
    render(() => <ChatsView />);
    expect(screen.getByText(WARNING)).toBeInTheDocument();
    const wire = daemon.chats.find((chat) => chat.id === worker.id);
    if (!wire) throw new Error("the daemon has no such chat");
    wire.permissionMode = "plan";
    daemon.emit("project:api", "chat.updated", { chat: wire });
    await vi.waitFor(() => expect(screen.queryByText(WARNING)).not.toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /^Integrator/ }));
    expect(M.S.chatOpen.api).toBe(system.id);
  });
});
