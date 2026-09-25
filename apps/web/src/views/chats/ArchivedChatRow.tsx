import type { Chat } from "~/mock";
import { M } from "~/mock";
import { ROW_OPEN_CLASS } from "./ChatRow";
import { ChatRowActions } from "./ChatRowActions";
import type { ChatListState } from "./use-chat-list-state";

export interface ArchivedChatRowProps {
  chat: Chat;
  list: ChatListState;
}

/** One archived chat: quieter than an active row, and not a list item, as in the design. */
export function ArchivedChatRow(props: ArchivedChatRowProps) {
  return (
    <div class="relative flex items-center rounded-md">
      <button
        type="button"
        onClick={() => props.list.open(props.chat)}
        class={`${ROW_OPEN_CLASS} text-secondary`}
      >
        <span class="truncate">{props.chat.title}</span>
        <span class="text-caption leading-4">{M.rel(props.chat.last)}</span>
      </button>
      <ChatRowActions chat={props.chat} list={props.list} archived />
    </div>
  );
}
