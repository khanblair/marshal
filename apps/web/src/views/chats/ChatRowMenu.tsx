import { Menu, MenuItem } from "@marshal/ui";
import { Show } from "solid-js";
import type { Chat } from "~/mock";
import type { ChatListState } from "./use-chat-list-state";

export interface ChatRowMenuProps {
  chat: Chat;
  list: ChatListState;
}

/**
 * The row's Rename, Archive or Restore, and Delete menu. It has no `onClose`: like the design, it
 * stays open until the trigger is pressed again or an item runs.
 */
export function ChatRowMenu(props: ChatRowMenuProps) {
  return (
    <Menu class="absolute top-[calc(100%-4px)] right-1 z-banner w-45">
      <MenuItem icon="pencil" onClick={() => props.list.startRename(props.chat.id)}>
        Rename
      </MenuItem>
      <Show
        when={props.chat.archived}
        fallback={
          <MenuItem icon="archive" onClick={() => props.list.archive(props.chat, true)}>
            Archive
          </MenuItem>
        }
      >
        <MenuItem icon="archive-restore" onClick={() => props.list.archive(props.chat, false)}>
          Restore
        </MenuItem>
      </Show>
      <MenuItem
        icon="trash-2"
        class="text-status-danger-text!"
        onClick={() => props.list.remove(props.chat)}
      >
        Delete
      </MenuItem>
    </Menu>
  );
}
