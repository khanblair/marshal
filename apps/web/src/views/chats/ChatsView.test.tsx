import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { ChatsView } from "./ChatsView";
import {
  chatByTitle,
  openButton,
  PHONE_PX,
  rowOf,
  rowTitles,
  showChats,
  threadKinds,
} from "./chats-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});
vi.mock("~/features/chat/ChatThread", async () => import("./test-thread-stub"));

const TABLET_PX = 820;
const searchBox = () => screen.getByRole("textbox", { name: "Search chats" });
const search = (text: string) => fireEvent.input(searchBox(), { target: { value: text } });
const openChat = () => M.S.chatOpen.api;

beforeEach(() => {
  vi.useFakeTimers();
  showChats("api");
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe("ChatsView list", () => {
  it("fills its parent, opts out of card navigation, and has the list and the chat side by side", () => {
    const { container } = render(() => <ChatsView />);
    expect(container.firstElementChild).toHaveAttribute("data-no-nav", "1");
    expect(container.firstElementChild).toHaveClass("absolute", "inset-0", "flex", "bg-surface");
    expect(screen.getByRole("complementary", { name: "Chats" })).toHaveClass("w-chat-list");
    expect(screen.getByRole("region", { name: "Upgrade grpc-go" })).toBeInTheDocument();
  });

  it("lists the active chats newest first and opens the newest one", () => {
    render(() => <ChatsView />);
    expect(rowTitles()).toEqual(["Upgrade grpc-go", "Rate limiting per key", "Load test results"]);
    expect(openChat()).toBe(chatByTitle("Upgrade grpc-go").id);
    expect(openButton("Upgrade grpc-go")).toHaveAttribute("aria-current", "page");
    expect(rowOf("Upgrade grpc-go")).toHaveClass("bg-surface-selected");
    expect(rowOf("Rate limiting per key")).toHaveClass("bg-transparent");
  });

  it("shows each chat's target and how long ago it was active", () => {
    render(() => <ChatsView />);
    const row = within(rowOf("Load test results"));
    expect(row.getByText("Tester role")).toBeInTheDocument();
    const chat = chatByTitle("Load test results");
    expect(row.getByText(M.rel(chat.last))).toHaveAttribute("title", M.full(chat.last));
    expect(within(rowOf("Upgrade grpc-go")).getByText("Orchestrator")).toBeInTheDocument();
  });

  it("names a card target after the card's agent", () => {
    showChats("web");
    render(() => <ChatsView />);
    expect(
      within(rowOf("Settings dark mode")).getByText(`#118 ${M.card("web#118")?.agent}`),
    ).toBeVisible();
  });

  it("opens another chat when its row is pressed, and shows its messages", () => {
    render(() => <ChatsView />);
    fireEvent.click(openButton("Rate limiting per key"));
    expect(openChat()).toBe(chatByTitle("Rate limiting per key").id);
    expect(screen.getByRole("region", { name: "Rate limiting per key" })).toBeInTheDocument();
    expect(threadKinds()).toEqual(["user", "agent", "card"]);
  });

  it("passes card links and approvals to the thread", () => {
    render(() => <ChatsView />);
    expect(threadKinds()).toEqual(["user", "card", "agent", "approval"]);
  });

  it("keeps the two-column layout on a tablet", () => {
    cleanup();
    showChats("api", TABLET_PX);
    render(() => <ChatsView />);
    expect(screen.getByRole("complementary", { name: "Chats" })).toHaveClass("w-chat-list");
    expect(screen.getByRole("region", { name: "Upgrade grpc-go" })).toBeInTheDocument();
  });
});

describe("opening a chat", () => {
  it("opens the first active chat only the first time", () => {
    M.S.chatOpen = { api: chatByTitle("Load test results").id };
    render(() => <ChatsView />);
    expect(openChat()).toBe(chatByTitle("Load test results").id);
    expect(screen.getByRole("region", { name: "Load test results" })).toBeInTheDocument();
  });

  it("shows the empty state when no chat is open", () => {
    M.S.chatOpen = { api: null };
    render(() => <ChatsView />);
    expect(screen.getByRole("region", { name: "Chat" })).toBeInTheDocument();
    expect(
      screen.getByText(
        "Pick a chat, or start a new one with the Orchestrator, a role, or a card's agent.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Message" })).not.toBeInTheDocument();
  });

  it("starts a chat from the empty state's New chat button", () => {
    M.S.chatOpen = { api: null };
    render(() => <ChatsView />);
    const [, quick] = screen.getAllByRole("button", { name: "New chat" });
    expect(quick).toHaveClass("hover:bg-ink!");
    fireEvent.click(quick as HTMLElement);
    expect(M.S.newChatOpen).toBe(true);
    expect(screen.getByText("Who do you want to talk to?")).toBeInTheDocument();
  });

  it("says when the project has no chats", () => {
    showChats("web");
    M.S.chats.web = [];
    render(() => <ChatsView />);
    expect(
      screen.getByText("No chats yet. Start one to plan work with the Orchestrator."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Archived/ })).toHaveTextContent("Archived0");
  });
});

describe("searching", () => {
  it("filters by title and by message text, ignoring case", () => {
    render(() => <ChatsView />);
    search("GRPC");
    expect(rowTitles()).toEqual(["Upgrade grpc-go"]);
    expect(M.S.chatQuery.api).toBe("GRPC");
    search("p99");
    expect(rowTitles()).toEqual(["Load test results"]);
  });

  it("says nothing matches and quotes the query as typed", () => {
    render(() => <ChatsView />);
    search("Zzz");
    expect(rowTitles()).toEqual([]);
    expect(screen.getByText('No chats match "Zzz".')).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Archived/ })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  it("opens the archived section by itself when an archived chat matches", () => {
    render(() => <ChatsView />);
    search("jwks");
    const toggle = screen.getByRole("button", { name: /^Archived/ });
    expect(toggle).toHaveTextContent("Archived1");
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(rowOf("JWKS caching question")).toBeInTheDocument();
    expect(rowTitles()).toEqual([]);
  });
});

describe("archived chats", () => {
  const toggle = () => screen.getByRole("button", { name: /^Archived/ });

  it("hides them until the section is opened, then lists them as plain rows", () => {
    render(() => <ChatsView />);
    expect(toggle()).toHaveTextContent("Archived1");
    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("JWKS caching question")).not.toBeInTheDocument();
    fireEvent.click(toggle());
    expect(M.S.archOpen.api).toBe(true);
    expect(toggle()).toHaveAttribute("aria-expanded", "true");
    expect(rowOf("JWKS caching question")).not.toHaveAttribute("role", "listitem");
    fireEvent.click(toggle());
    expect(screen.queryByText("JWKS caching question")).not.toBeInTheDocument();
  });

  it("says when there are none", () => {
    showChats("web");
    render(() => <ChatsView />);
    fireEvent.click(toggle());
    expect(screen.getByText("No archived chats.")).toBeInTheDocument();
  });

  it("opens an archived chat with a Restore button that brings it back", () => {
    render(() => <ChatsView />);
    fireEvent.click(toggle());
    fireEvent.click(openButton("JWKS caching question"));
    const pane = within(screen.getByRole("region", { name: "JWKS caching question" }));
    fireEvent.click(pane.getByRole("button", { name: "Restore" }));
    expect(chatByTitle("JWKS caching question").archived).toBe(false);
    expect(M.S.toasts.at(-1)?.msg).toBe("Chat restored");
    expect(pane.queryByRole("button", { name: "Restore" })).not.toBeInTheDocument();
    expect(rowTitles()).toContain("JWKS caching question");
  });
});

describe("on a phone", () => {
  beforeEach(() => {
    cleanup();
    showChats("api", PHONE_PX);
  });

  it("shows the list first, full width, with no chat open", () => {
    render(() => <ChatsView />);
    expect(openChat()).toBeNull();
    expect(screen.getByRole("complementary", { name: "Chats" })).toHaveClass("w-full");
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
    expect(rowTitles()).toHaveLength(3);
  });

  it("shows the chat in place of the list, and Back returns to the list", () => {
    render(() => <ChatsView />);
    fireEvent.click(openButton("Load test results"));
    expect(screen.queryByRole("complementary")).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Load test results" })).toBeInTheDocument();
    expect(screen.queryByText(/Enter sends/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Back to chats" }));
    expect(openChat()).toBeNull();
    expect(screen.getByRole("complementary", { name: "Chats" })).toBeInTheDocument();
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
  });

  it("keeps the draft when the reader goes back to the list and returns", () => {
    render(() => <ChatsView />);
    fireEvent.click(openButton("Load test results"));
    fireEvent.input(screen.getByRole("textbox", { name: "Message" }), {
      target: { value: "Half a thought" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Back to chats" }));
    fireEvent.click(openButton("Rate limiting per key"));
    expect(screen.getByRole("textbox", { name: "Message" })).toHaveValue("Half a thought");
  });
});
