import { cx, IconLabel } from "@marshal/ui";
import { Show } from "solid-js";
import type { Chat } from "~/mock";
import { M } from "~/mock";
import { ChatRowActions } from "./ChatRowActions";
import { targetIcon, targetLabel } from "./chat-model";
import { RenameInput } from "./RenameInput";
import type { ChatListState } from "./use-chat-list-state";

/** The button that opens a chat, shared by active and archived rows. */
export const ROW_OPEN_CLASS =
  "flex min-w-0 flex-1 flex-col gap-0.5 rounded-md border-none bg-transparent pt-2 pr-1 pb-2 pl-2.5 text-left hover:bg-surface-hover";

const TARGET_ICON_PX = 12;

export interface ChatRowProps {
  chat: Chat;
  list: ChatListState;
}

/** One active chat in the list: title, target and time, a More actions menu, or a rename field. */
export function ChatRow(props: ChatRowProps) {
  const current = () => props.list.currentId() === props.chat.id;
  return (
    // biome-ignore lint/a11y/useSemanticElements: the design uses a div with a listitem role; a li brings list styles, and Tailwind preflight is off
    <div
      role="listitem"
      class={cx(
        "relative flex items-center rounded-md",
        current() ? "bg-surface-selected" : "bg-transparent",
      )}
    >
      <Show
        when={props.list.renamingId() === props.chat.id}
        fallback={
          <>
            <button
              type="button"
              onClick={() => props.list.open(props.chat)}
              aria-current={current() ? "page" : undefined}
              class={ROW_OPEN_CLASS}
            >
              <span class="truncate font-semibold">{props.chat.title}</span>
              <span class="flex flex-wrap gap-x-2.5 gap-y-0 text-caption leading-4 text-secondary">
                <IconLabel icon={targetIcon(props.chat.target)} size={TARGET_ICON_PX}>
                  {targetLabel(props.chat.target)}
                </IconLabel>
                <span title={M.full(props.chat.last)}>{M.rel(props.chat.last)}</span>
              </span>
            </button>
            <ChatRowActions chat={props.chat} list={props.list} />
          </>
        }
      >
        <RenameInput chat={props.chat} list={props.list} />
      </Show>
    </div>
  );
}
