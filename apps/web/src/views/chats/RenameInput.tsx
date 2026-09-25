import { Input } from "@marshal/ui";
import { untrack } from "solid-js";
import type { Chat } from "~/mock";
import type { ChatListState } from "./use-chat-list-state";

export interface RenameInputProps {
  chat: Chat;
  list: ChatListState;
}

/**
 * The field that replaces a row while it is renamed. It is uncontrolled: the name is set once.
 * Enter and blur keep the name, Escape cancels and does not reach the app's Escape handler.
 */
export function RenameInput(props: RenameInputProps) {
  const initial = untrack(() => props.chat.title);
  const finish = (value: string | null) => props.list.finishRename(props.chat, value);
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === "Enter") {
      event.preventDefault();
      finish((event.target as HTMLInputElement).value);
    } else if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      finish(null);
    }
  };
  return (
    <Input
      // A native listener runs before the window listener of the app's keyboard handler.
      ref={(el) => {
        el.addEventListener("keydown", onKeyDown);
        setTimeout(() => {
          el.focus();
          el.select();
        }, 0);
      }}
      value={initial}
      onBlur={(event) => finish(event.currentTarget.value)}
      aria-label="Chat name"
      class="m-1 h-9! min-w-0 flex-1 px-2! py-0 font-semibold"
    />
  );
}
