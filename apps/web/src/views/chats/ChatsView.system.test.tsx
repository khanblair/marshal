import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Chat } from "~/mock";
import { M } from "~/mock";
import { ChatsView } from "./ChatsView";
import { chatByTitle, rowTitles, showChats } from "./chats-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});
vi.mock("~/features/chat/ChatThread", async () => import("./test-thread-stub"));

/*
 * The pinned Integrator chat and the warning about a chat that edits the owner's folder, on the
 * store's chats. The daemon's own refusals of a rename, an archive, and a delete are in
 * ChatsView.system.daemon.test.tsx.
 */

const WARNING = "This chat edits your folder directly.";
const FIRST_SEEDED = "Upgrade grpc-go";

/** The Integrator chat, older than every seeded chat, as a quiet system chat is. */
const integrator = (): Chat => ({
  id: "sys-integrator",
  pid: "api",
  title: "Integrator",
  target: "Integrator",
  system: "integrator",
  mode: "auto-edits",
  msgs: [],
  last: 1,
  archived: false,
});

const seedWith = (...extra: Chat[]) => {
  M.S.chats.api = [...(M.S.chats.api ?? []), ...extra];
};
let madeCount = 0;
const addChat = (over: Partial<Chat>): Chat => {
  madeCount += 1;
  const made: Chat = {
    id: `made-${madeCount}`,
    pid: "api",
    title: "Made chat",
    target: "Worker",
    msgs: [],
    last: 2,
    archived: false,
    ...over,
  };
  seedWith(made);
  return made;
};
const menuButton = (title: string) =>
  screen.queryByRole("button", { name: `More actions for ${title}` });

beforeEach(() => {
  vi.useFakeTimers();
  showChats("api");
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe("the Integrator chat in the list", () => {
  it("is first, even though every other chat is more recent, and carries an Integrator badge", () => {
    seedWith(integrator());
    render(() => <ChatsView />);
    const list = within(screen.getByRole("complementary", { name: "Chats" }));
    expect(list.getAllByRole("listitem")[0]).toHaveTextContent("Integrator");
    expect(M.chatsOf("api")[0]?.system).toBe("integrator");
    const row = list.getAllByRole("listitem")[0] as HTMLElement;
    expect(
      within(row).getByText("Integrator", { selector: "span.rounded-xs" }),
    ).toBeInTheDocument();
    // The chats after it are in their usual order, most recent first.
    expect(rowTitles().slice(1, 3)).toEqual(
      M.chatsOf("api")
        .filter((chat) => !chat.system && !chat.archived)
        .slice(0, 2)
        .map((chat) => chat.title),
    );
  });

  it("has no More actions button, because it cannot be renamed, archived, or deleted", () => {
    seedWith(integrator());
    render(() => <ChatsView />);
    expect(menuButton("Integrator")).toBeNull();
    expect(menuButton(FIRST_SEEDED)).toBeInTheDocument();
  });

  it("says where it works, in its header", () => {
    seedWith(integrator());
    M.S.chatOpen = { api: "sys-integrator" };
    render(() => <ChatsView />);
    const header = screen.getByRole("region", { name: "Integrator" });
    expect(within(header).getByText("Works in its own workspace")).toBeInTheDocument();
    expect(within(header).getByText("Integrator role")).toBeInTheDocument();
    expect(screen.queryByText(WARNING)).not.toBeInTheDocument();
  });

  it("opens by itself only when it is the project's one chat", () => {
    M.S.chats.api = [integrator()];
    render(() => <ChatsView />);
    expect(M.S.chatOpen.api).toBe("sys-integrator");
    cleanup();

    showChats("api");
    seedWith(integrator());
    render(() => <ChatsView />);
    expect(M.S.chatOpen.api).toBe(chatByTitle(FIRST_SEEDED).id);
  });

  it("opens when it is pressed", () => {
    seedWith(integrator());
    render(() => <ChatsView />);
    fireEvent.click(screen.getByRole("button", { name: /^Integrator/ }));
    expect(M.S.chatOpen.api).toBe("sys-integrator");
  });
});

describe("the warning in a chat's header", () => {
  const open = (chat: Chat) => {
    M.S.chatOpen = { api: chat.id };
    render(() => <ChatsView />);
  };

  it("is shown for a chat that runs in the owner's folder with a mode that edits without asking", () => {
    for (const mode of ["auto-edits", "full-auto", "bypass"]) {
      const chat = addChat({ title: `Worker in ${mode}`, mode });
      open(chat);
      expect(screen.getByRole("note")).toHaveTextContent(WARNING);
      cleanup();
    }
  });

  it("is not shown when the mode asks first, reads only, or is not known", () => {
    for (const mode of ["ask", "plan"]) {
      open(addChat({ title: `Worker in ${mode}`, mode }));
      expect(screen.queryByText(WARNING), mode).not.toBeInTheDocument();
      cleanup();
    }
    open(addChat({ title: "A mock chat" }));
    expect(screen.queryByText(WARNING)).not.toBeInTheDocument();
  });

  it("is not shown for an Orchestrator chat, which is plan-only whatever mode it was made with", () => {
    open(addChat({ title: "Plan the export", target: "Orchestrator", mode: "auto-edits" }));
    expect(screen.queryByText(WARNING)).not.toBeInTheDocument();
  });

  it("is not shown for the Integrator chat, which works in its own workspace", () => {
    const chat = integrator();
    seedWith(chat);
    open(chat);
    expect(screen.queryByText(WARNING)).not.toBeInTheDocument();
  });

  it("is shown again for another chat once it is open, and goes with the chat that is closed", () => {
    const chat = addChat({ title: "Fix the flaky test", mode: "full-auto" });
    seedWith(integrator());
    open(chat);
    expect(screen.getByText(WARNING)).toBeInTheDocument();
    M.openChat("api", "sys-integrator");
    expect(screen.queryByText(WARNING)).not.toBeInTheDocument();
  });
});
