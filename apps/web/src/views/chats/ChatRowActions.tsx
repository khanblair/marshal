import { cx, IconButton } from "@marshal/ui";
import { Show } from "solid-js";
import type { Chat } from "~/mock";
import { ChatRowMenu } from "./ChatRowMenu";
import type { ChatListState } from "./use-chat-list-state";

export interface ChatRowActionsProps {
  chat: Chat;
  list: ChatListState;
  /** Archived rows have a menu button that does not react to hover, as in the design. */
  archived?: boolean;
}

const ICON_PX = 14;

/** The row's More actions button and its menu. Place it inside the row, which is `relative`. */
export function ChatRowActions(props: ChatRowActionsProps) {
  const open = () => props.list.menuId() === props.chat.id;
  return (
    <>
      <IconButton
        label={`More actions for ${props.chat.title}`}
        icon="ellipsis-vertical"
        iconSize={ICON_PX}
        tone="muted"
        aria-expanded={open()}
        class={cx("mr-1", props.archived && "hover:bg-transparent! hover:text-muted!")}
        onClick={(event) => props.list.toggleMenu(props.chat.id, event)}
      />
      <Show when={open()}>
        <ChatRowMenu chat={props.chat} list={props.list} />
      </Show>
    </>
  );
}
