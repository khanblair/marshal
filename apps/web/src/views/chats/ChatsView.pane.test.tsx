import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { ChatsView } from "./ChatsView";
import {
  chatByTitle,
  openButton,
  PHONE_PX,
  REPLY_DONE_MS,
  showChats,
  threadKinds,
} from "./chats-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});
vi.mock("~/features/chat/ChatThread", async () => import("./test-thread-stub"));

const FOCUS_DELAY_MS = 50;
const SCROLL_HEIGHT_PX = 2000;
const VIEW_HEIGHT_PX = 500;
const composer = () => screen.getByRole("textbox", { name: "Message" });
const sendButton = () => screen.getByRole("button", { name: "Send message" });
const threadText = () => screen.getByTestId("thread").textContent ?? "";
const type = (text: string) => fireEvent.input(composer(), { target: { value: text } });

beforeEach(() => {
  vi.useFakeTimers();
  showChats("api");
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe("chat header", () => {
  it("shows the title and the Orchestrator's facts", () => {
    render(() => <ChatsView />);
    const pane = within(screen.getByRole("region", { name: "Upgrade grpc-go" }));
    expect(pane.getByText("Upgrade grpc-go")).toBeInTheDocument();
    for (const bit of ["Orchestrator", "claude-opus-4-1", "High thinking"]) {
      expect(pane.getByText(bit)).toBeInTheDocument();
    }
    expect(pane.queryByRole("button", { name: "Restore" })).not.toBeInTheDocument();
    expect(pane.queryByRole("button", { name: "Back to chats" })).not.toBeInTheDocument();
  });

  it("shows a role's facts, and a card agent's facts", () => {
    render(() => <ChatsView />);
    fireEvent.click(openButton("Load test results"));
    expect(screen.getByText("Tester role", { selector: "section span" })).toBeInTheDocument();
    expect(screen.getByText("Uses the role template")).toBeInTheDocument();
    cleanup();
    showChats("web");
    render(() => <ChatsView />);
    const card = M.card(118);
    expect(screen.getByRole("region", { name: "Settings dark mode" })).toBeInTheDocument();
    expect(screen.getByText(card?.model ?? "")).toBeInTheDocument();
    expect(screen.getByText(card?.asleep ? "Asleep" : "Awake")).toBeInTheDocument();
  });
});

describe("composer", () => {
  it("asks who the message goes to and explains the keys", () => {
    render(() => <ChatsView />);
    expect(composer()).toHaveAttribute("placeholder", "Message the Orchestrator");
    expect(sendButton()).toBeDisabled();
    expect(
      screen.getByText(
        `Enter sends. Shift and Enter adds a new line. Cards created here appear on the ${M.proj("api")?.name} board.`,
      ),
    ).toBeInTheDocument();
    fireEvent.click(openButton("Load test results"));
    expect(composer()).toHaveAttribute("placeholder", "Message the Tester");
  });

  it("sends with Enter, clears the draft, and gets the scripted answer", () => {
    render(() => <ChatsView />);
    type("How is it going");
    expect(sendButton()).toBeEnabled();
    fireEvent.keyDown(composer(), { key: "Enter", shiftKey: true });
    expect(threadText()).not.toContain("How is it going");
    fireEvent.keyDown(composer(), { key: "Enter" });
    expect(composer()).toHaveValue("");
    expect(sendButton()).toBeDisabled();
    expect(threadKinds().at(-1)).toBe("user");
    expect(threadText()).toContain("How is it going");
    vi.advanceTimersByTime(REPLY_DONE_MS);
    expect(threadKinds().at(-1)).toBe("agent");
    expect(threadText()).toContain("cards are working");
  });

  it("does not send a blank draft", () => {
    render(() => <ChatsView />);
    type("   ");
    expect(sendButton()).toBeDisabled();
    fireEvent.keyDown(composer(), { key: "Enter" });
    expect(threadKinds()).toEqual(["user", "card", "agent", "approval"]);
  });

  it("sends a suggestion as it is and leaves the draft cleared", () => {
    render(() => <ChatsView />);
    type("half typed");
    fireEvent.click(screen.getByRole("button", { name: "What is blocked?" }));
    expect(composer()).toHaveValue("");
    expect(threadText()).toContain("What is blocked?");
    vi.advanceTimersByTime(REPLY_DONE_MS);
    expect(threadKinds().slice(-3)).toEqual(["approval", "user", "links"]);
    expect(threadText()).toContain("waiting on you");
  });

  it("offers the three suggestions", () => {
    render(() => <ChatsView />);
    for (const label of [
      "What is blocked?",
      "Make cards for the export work",
      "Merge the next ready card",
    ]) {
      expect(screen.getByRole("button", { name: label })).toHaveClass("rounded-full");
    }
  });

  it("names a chat after its first message", () => {
    render(() => <ChatsView />);
    fireEvent.click(screen.getAllByRole("button", { name: "New chat" })[0] as HTMLElement);
    fireEvent.submit(screen.getByText("Start chat").closest("form") as HTMLFormElement);
    expect(screen.getByRole("region", { name: "New chat" })).toBeInTheDocument();
    type("Plan the billing export. Then more words");
    fireEvent.keyDown(composer(), { key: "Enter" });
    expect(
      screen.getByRole("region", { name: "Plan the billing export. Then more" }),
    ).toBeVisible();
  });
});

describe("New chat", () => {
  const openForm = () =>
    fireEvent.click(screen.getAllByRole("button", { name: "New chat" })[0] as HTMLElement);

  it("opens a popover under the search with the Orchestrator chosen", () => {
    render(() => <ChatsView />);
    const toggle = screen.getAllByRole("button", { name: "New chat" })[0] as HTMLElement;
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    const select = screen.getByRole("combobox");
    expect(select).toHaveValue("Orchestrator");
    expect(select.closest("form")).toHaveClass("absolute", "top-9.5");
    expect(
      screen.getByText("The Orchestrator plans work and creates cards on this project's board."),
    ).toBeInTheDocument();
    const options = within(select).getAllByRole("option");
    expect(options[0]).toHaveTextContent("Orchestrator");
    expect(options[1]).toHaveTextContent("Worker role");
    expect(options.at(-1)?.textContent).toMatch(/^#\d+ /);
  });

  it("closes with Cancel or the New chat button", () => {
    render(() => <ChatsView />);
    openForm();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(M.S.newChatOpen).toBe(false);
    expect(screen.queryByText("Start chat")).not.toBeInTheDocument();
    openForm();
    openForm();
    expect(screen.queryByText("Start chat")).not.toBeInTheDocument();
  });

  it("starts a chat with the chosen role, opens it, and focuses its message box", () => {
    render(() => <ChatsView />);
    openForm();
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "Tester" } });
    fireEvent.click(screen.getByRole("button", { name: "Start chat" }));
    expect(M.S.newChatOpen).toBe(false);
    const started = M.S.chats.api?.at(-1);
    expect(started).toMatchObject({ title: "New chat", target: "Tester", archived: false });
    expect(M.S.chatOpen.api).toBe(started?.id);
    expect(screen.getByRole("region", { name: "New chat" })).toBeInTheDocument();
    expect(screen.getByText("Tester role", { selector: "section span" })).toBeInTheDocument();
    expect(
      screen.getByText("Say what you want in plain words. The first message names this chat."),
    ).toBeInTheDocument();
    expect(composer()).not.toHaveFocus();
    vi.advanceTimersByTime(FOCUS_DELAY_MS);
    expect(composer()).toHaveFocus();
  });

  it("can talk to a card's agent", () => {
    render(() => <ChatsView />);
    openForm();
    const cardOption = within(screen.getByRole("combobox"))
      .getAllByRole("option")
      .find((option) => option.textContent?.startsWith("#"));
    fireEvent.change(screen.getByRole("combobox"), {
      target: { value: cardOption?.getAttribute("value") },
    });
    fireEvent.click(screen.getByRole("button", { name: "Start chat" }));
    const started = M.S.chats.api?.at(-1);
    expect(started?.target).toMatch(/^#\d+$/);
    expect(composer()).toHaveAttribute("placeholder", `Message ${started?.target}`);
  });

  it("is a bottom sheet on a phone, and the new chat replaces the list", () => {
    cleanup();
    showChats("api", PHONE_PX);
    render(() => <ChatsView />);
    openForm();
    expect(screen.getByRole("combobox").closest("form")).toHaveClass(
      "fixed",
      "bottom-0",
      "z-sheet",
    );
    fireEvent.click(screen.getByRole("button", { name: "Start chat" }));
    expect(screen.queryByRole("complementary")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Back to chats" })).toBeInTheDocument();
    vi.advanceTimersByTime(FOCUS_DELAY_MS);
    expect(composer()).toHaveFocus();
    expect(M.S.newChatOpen).toBe(false);
  });
});

describe("thread of the open chat", () => {
  it("follows the chat that is open", () => {
    render(() => <ChatsView />);
    expect(threadText()).toContain("upgrade grpc-go");
    fireEvent.click(openButton("Rate limiting per key"));
    expect(threadText()).toContain("rate limiting");
    expect(chatByTitle("Rate limiting per key").msgs).toHaveLength(3);
  });

  it("shows Jump to latest when an answer arrives while the reader is scrolled up", () => {
    render(() => <ChatsView />);
    const box = screen.getByTestId("thread").parentElement?.parentElement as HTMLElement;
    let top = 0;
    Object.defineProperties(box, {
      scrollHeight: { configurable: true, value: SCROLL_HEIGHT_PX },
      clientHeight: { configurable: true, value: VIEW_HEIGHT_PX },
      scrollTop: { configurable: true, get: () => top, set: (value: number) => (top = value) },
    });
    type("How is it going");
    fireEvent.keyDown(composer(), { key: "Enter" });
    vi.advanceTimersByTime(0);
    expect(top).toBe(SCROLL_HEIGHT_PX);
    top = 0;
    fireEvent.scroll(box);
    expect(screen.queryByRole("button", { name: "Jump to latest" })).not.toBeInTheDocument();
    vi.advanceTimersByTime(REPLY_DONE_MS);
    fireEvent.click(screen.getByRole("button", { name: "Jump to latest" }));
    expect(top).toBe(SCROLL_HEIGHT_PX);
    expect(screen.queryByRole("button", { name: "Jump to latest" })).not.toBeInTheDocument();
  });
});
