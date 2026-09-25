import { cx } from "@marshal/ui";
import { M } from "~/mock";
import { ChatListBody } from "./ChatListBody";
import { ChatListHeader } from "./ChatListHeader";
import type { ChatListState } from "./use-chat-list-state";

export interface ChatListProps {
  list: ChatListState;
  /** Called after a chat is started from the New chat form. */
  onStarted: () => void;
}

/** The chats of the current project: search, New chat, and the list. */
export function ChatList(props: ChatListProps) {
  return (
    <aside
      aria-label="Chats"
      class={cx(
        "flex min-w-0 flex-none flex-col border-r border-border bg-canvas",
        M.mobile ? "w-full" : "w-chat-list",
      )}
    >
      <ChatListHeader onStarted={props.onStarted} />
      <ChatListBody list={props.list} />
    </aside>
  );
}
