import { createSignal } from "solid-js";
import type { Chat } from "~/mock";
import { M } from "~/mock";

/** What the chat rows share: which row menu is open, which row is being renamed, and the actions. */
export interface ChatListState {
  pid: () => string;
  /** The id of the chat open in the pane. */
  currentId: () => string | null;
  menuId: () => string | null;
  renamingId: () => string | null;
  open: (chat: Chat) => void;
  toggleMenu: (id: string, event: MouseEvent) => void;
  startRename: (id: string) => void;
  /** Ends a rename. `null` cancels; a changed name renames. Later calls for the same rename do nothing. */
  finishRename: (chat: Chat, value: string | null) => void;
  archive: (chat: Chat, on: boolean) => void;
  remove: (chat: Chat) => void;
}

/**
 * The list's local state and actions, as in the design. The menu and rename state is local to
 * each list, so it is not in the store. `onOpen` runs when a chat is opened from the list.
 */
export function useChatListState(pid: () => string, onOpen: () => void): ChatListState {
  const [menuId, setMenuId] = createSignal<string | null>(null);
  const [renamingId, setRenamingId] = createSignal<string | null>(null);
  return {
    pid,
    currentId: () => M.S.chatOpen[pid()] ?? null,
    menuId,
    renamingId,
    open: (chat) => {
      M.openChat(pid(), chat.id);
      setMenuId(null);
      onOpen();
    },
    toggleMenu: (id, event) => {
      event.stopPropagation();
      setMenuId(menuId() === id ? null : id);
    },
    startRename: (id) => {
      setMenuId(null);
      setRenamingId(id);
    },
    finishRename: (chat, value) => {
      if (renamingId() !== chat.id) return;
      setRenamingId(null);
      if (value !== null && value !== chat.title) M.renameChat(pid(), chat.id, value);
    },
    archive: (chat, on) => {
      setMenuId(null);
      M.archiveChat(pid(), chat.id, on);
    },
    remove: (chat) => {
      setMenuId(null);
      M.deleteChat(pid(), chat.id);
    },
  };
}
