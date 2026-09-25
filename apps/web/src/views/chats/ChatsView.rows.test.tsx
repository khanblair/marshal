import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { ChatsView } from "./ChatsView";
import { chatByTitle, openButton, rowOf, rowTitles, showChats } from "./chats-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});
vi.mock("~/features/chat/ChatThread", async () => import("./test-thread-stub"));

const ROW_TITLE = "Rate limiting per key";
const OPEN_TITLE = "Upgrade grpc-go";
const menuButton = (title: string) =>
  within(rowOf(title)).getByRole("button", { name: `More actions for ${title}` });
const openMenu = (title: string) => fireEvent.click(menuButton(title));
const item = (name: string) => screen.getByRole("menuitem", { name });
const renameField = () => screen.getByRole("textbox", { name: "Chat name" });
const startRename = (title: string) => {
  openMenu(title);
  fireEvent.click(item("Rename"));
  vi.advanceTimersByTime(0);
};
const toggleArchived = () => fireEvent.click(screen.getByRole("button", { name: /^Archived/ }));

let errors: ReturnType<typeof vi.spyOn>;
beforeEach(() => {
  vi.useFakeTimers();
  errors = vi.spyOn(console, "error").mockImplementation(() => {});
  showChats("api");
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
  errors.mockRestore();
});

describe("row menu", () => {
  it("opens with Rename, Archive, and Delete, and closes when its button is pressed again", () => {
    render(() => <ChatsView />);
    expect(menuButton(ROW_TITLE)).toHaveAttribute("aria-expanded", "false");
    openMenu(ROW_TITLE);
    expect(menuButton(ROW_TITLE)).toHaveAttribute("aria-expanded", "true");
    expect(screen.getAllByRole("menuitem").map((el) => el.textContent)).toEqual([
      "Rename",
      "Archive",
      "Delete",
    ]);
    expect(screen.getByRole("menu")).toHaveClass("absolute", "w-45", "z-banner");
    expect(item("Delete")).toHaveClass("text-status-danger-text!");
    openMenu(ROW_TITLE);
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("keeps one menu open at a time", () => {
    render(() => <ChatsView />);
    openMenu(ROW_TITLE);
    openMenu("Load test results");
    expect(screen.getAllByRole("menu")).toHaveLength(1);
    expect(menuButton(ROW_TITLE)).toHaveAttribute("aria-expanded", "false");
    expect(menuButton("Load test results")).toHaveAttribute("aria-expanded", "true");
  });

  it("does not open the chat when its menu button is pressed", () => {
    render(() => <ChatsView />);
    openMenu(ROW_TITLE);
    expect(M.S.chatOpen.api).toBe(chatByTitle(OPEN_TITLE).id);
  });

  it("closes when a chat is opened from the list", () => {
    render(() => <ChatsView />);
    openMenu(ROW_TITLE);
    fireEvent.click(openButton("Load test results"));
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("has Restore instead of Archive on an archived chat", () => {
    render(() => <ChatsView />);
    toggleArchived();
    const trigger = menuButton("JWKS caching question");
    expect(trigger).toHaveClass("hover:bg-transparent!", "hover:text-muted!");
    fireEvent.click(trigger);
    expect(screen.getAllByRole("menuitem").map((el) => el.textContent)).toEqual([
      "Rename",
      "Restore",
      "Delete",
    ]);
    fireEvent.click(item("Restore"));
    expect(chatByTitle("JWKS caching question").archived).toBe(false);
    expect(M.S.toasts.at(-1)?.msg).toBe("Chat restored");
    expect(rowTitles()).toContain("JWKS caching question");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });
});

describe("archiving", () => {
  it("moves a chat to Archived and offers Undo", () => {
    render(() => <ChatsView />);
    openMenu(ROW_TITLE);
    fireEvent.click(item("Archive"));
    expect(chatByTitle(ROW_TITLE).archived).toBe(true);
    expect(rowTitles()).toEqual([OPEN_TITLE, "Load test results"]);
    expect(screen.getByRole("button", { name: /^Archived/ })).toHaveTextContent("Archived2");
    expect(M.S.chatOpen.api).toBe(chatByTitle(OPEN_TITLE).id);
    const toast = M.S.toasts.at(-1);
    expect(toast?.msg).toBe("Chat archived");
    expect(toast?.action?.label).toBe("Undo");
    toast?.action?.run();
    expect(rowTitles()).toEqual([OPEN_TITLE, ROW_TITLE, "Load test results"]);
  });

  it("closes the open chat when it is archived, and Undo brings the chat back to the list", () => {
    render(() => <ChatsView />);
    openMenu(OPEN_TITLE);
    fireEvent.click(item("Archive"));
    expect(M.S.chatOpen.api).toBeNull();
    expect(screen.getByRole("region", { name: "Chat" })).toBeInTheDocument();
    expect(screen.getByText(/^Pick a chat/)).toBeInTheDocument();
    M.S.toasts.at(-1)?.action?.run();
    expect(rowTitles()).toContain(OPEN_TITLE);
    expect(errors).not.toHaveBeenCalled();
  });
});

describe("deleting", () => {
  it("asks first, then removes the chat and says so", () => {
    render(() => <ChatsView />);
    openMenu(ROW_TITLE);
    fireEvent.click(item("Delete"));
    expect(M.S.dialog).toMatchObject({
      title: "Delete chat",
      message: `This deletes "${ROW_TITLE}" and its messages. Cards it created stay on the board.`,
      action: "Delete chat",
      destructive: true,
    });
    expect(rowTitles()).toContain(ROW_TITLE);
    M.S.dialog?.run();
    expect(rowTitles()).toEqual([OPEN_TITLE, "Load test results"]);
    expect(M.S.toasts.at(-1)?.msg).toBe("Chat deleted");
  });

  it("closes the pane when the open chat is deleted", () => {
    render(() => <ChatsView />);
    openMenu(OPEN_TITLE);
    fireEvent.click(item("Delete"));
    M.S.dialog?.run();
    expect(M.S.chatOpen.api).toBeNull();
    expect(screen.getByText(/^Pick a chat/)).toBeInTheDocument();
    expect(errors).not.toHaveBeenCalled();
  });
});

describe("renaming in place", () => {
  it("replaces the row with a focused, selected field holding the title", () => {
    render(() => <ChatsView />);
    startRename(ROW_TITLE);
    const field = renameField() as HTMLInputElement;
    expect(field).toHaveValue(ROW_TITLE);
    expect(field).toHaveFocus();
    expect(field.selectionStart).toBe(0);
    expect(field.selectionEnd).toBe(ROW_TITLE.length);
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    const row = field.closest<HTMLElement>('[role="listitem"]');
    expect(row).not.toBeNull();
    expect(within(row as HTMLElement).queryByRole("button")).not.toBeInTheDocument();
  });

  it("keeps the new name on Enter", () => {
    render(() => <ChatsView />);
    startRename(ROW_TITLE);
    fireEvent.input(renameField(), { target: { value: "Per key limits" } });
    fireEvent.keyDown(renameField(), { key: "Enter" });
    expect(chatByTitle("Per key limits").id).toBeTruthy();
    expect(screen.queryByRole("textbox", { name: "Chat name" })).not.toBeInTheDocument();
    expect(rowTitles()).toContain("Per key limits");
  });

  it("keeps the new name on blur", () => {
    render(() => <ChatsView />);
    startRename(ROW_TITLE);
    fireEvent.input(renameField(), { target: { value: "  Spaced out  " } });
    fireEvent.blur(renameField());
    expect(chatByTitle("Spaced out").archived).toBe(false);
  });

  it("renames once when Enter is followed by the blur that removing the field causes", () => {
    const rename = vi.spyOn(M, "renameChat");
    render(() => <ChatsView />);
    startRename(ROW_TITLE);
    const field = renameField();
    fireEvent.input(field, { target: { value: "Once" } });
    fireEvent.keyDown(field, { key: "Enter" });
    fireEvent.blur(field);
    expect(rename).toHaveBeenCalledTimes(1);
    rename.mockRestore();
  });

  it("does not rename when the name is unchanged", () => {
    const rename = vi.spyOn(M, "renameChat");
    render(() => <ChatsView />);
    startRename(ROW_TITLE);
    fireEvent.keyDown(renameField(), { key: "Enter" });
    expect(rename).not.toHaveBeenCalled();
    rename.mockRestore();
  });

  it("cancels on Escape without reaching the app's Escape handler", () => {
    const appKey = vi.fn();
    window.addEventListener("keydown", appKey);
    render(() => <ChatsView />);
    startRename(ROW_TITLE);
    fireEvent.input(renameField(), { target: { value: "Never used" } });
    fireEvent.keyDown(renameField(), { key: "Escape" });
    expect(appKey).not.toHaveBeenCalled();
    expect(rowTitles()).toContain(ROW_TITLE);
    expect(screen.queryByRole("textbox", { name: "Chat name" })).not.toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("textbox", { name: "Search chats" }), { key: "a" });
    expect(appKey).toHaveBeenCalledTimes(1);
    window.removeEventListener("keydown", appKey);
  });

  it("keeps the old name and says so when the new name is empty", () => {
    render(() => <ChatsView />);
    startRename(ROW_TITLE);
    fireEvent.input(renameField(), { target: { value: "   " } });
    fireEvent.keyDown(renameField(), { key: "Enter" });
    expect(M.S.toasts.at(-1)?.msg).toBe("Chat names can't be empty. The old name is kept.");
    expect(rowTitles()).toContain(ROW_TITLE);
  });
});
