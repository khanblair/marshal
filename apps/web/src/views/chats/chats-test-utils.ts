import { screen, within } from "@solidjs/testing-library";
import type { Chat } from "~/mock";
import { M } from "~/mock";

const seedChats: Record<string, Chat[]> = JSON.parse(JSON.stringify(M.S.chats));

const DESKTOP_PX = 1440;
export const PHONE_PX = 390;
const HEIGHT_PX = 900;
/** Longer than the reply delay plus the streaming of any scripted answer. */
export const REPLY_DONE_MS = 4000;

/** The chats view of `pid`, with the seeded chats and nothing open, searched, or pending. */
export function showChats(pid = "api", width = DESKTOP_PX): void {
  M.S.chats = JSON.parse(JSON.stringify(seedChats));
  M.S.chatOpen = {};
  M.S.chatQuery = {};
  M.S.archOpen = {};
  M.S.newChatOpen = false;
  M.S.dialog = null;
  M.S.toasts = [];
  M.setViewport(width, HEIGHT_PX);
  M.go("project", pid, "chat");
}

export function chatByTitle(title: string, pid = "api"): Chat {
  const found = M.S.chats[pid]?.find((chat) => chat.title === title);
  if (!found) throw new Error(`no chat titled ${title}`);
  return found;
}

/** The list item (or archived row) that shows `title`. */
export function rowOf(title: string): HTMLElement {
  const button = within(screen.getByRole("complementary", { name: "Chats" })).getByText(title);
  const row = button.closest("div");
  if (!row) throw new Error(`no row for ${title}`);
  return row;
}

/** The button that opens a chat (not its More actions button). */
export const openButton = (title: string): HTMLElement =>
  within(rowOf(title)).getByRole("button", { name: new RegExp(`^${title}`) });

export const rowTitles = (): string[] =>
  screen.queryAllByRole("listitem").map((item) => item.querySelector("span")?.textContent ?? "");

export const threadKinds = (): string[] =>
  Array.from(screen.getByTestId("thread").children).map(
    (child) => (child as HTMLElement).dataset.kind ?? "",
  );
