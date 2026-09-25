import { Icon } from "@marshal/ui";
import { For, Show } from "solid-js";
import type { Chat } from "~/mock";
import { M } from "~/mock";
import { ArchivedChatRow } from "./ArchivedChatRow";
import type { ChatListState } from "./use-chat-list-state";

export interface ArchivedSectionProps {
  /** The archived chats that match the search. */
  chats: Chat[];
  list: ChatListState;
  /** A search with matches opens the section by itself. */
  searching: boolean;
}

const CHEVRON_PX = 14;

/** The Archived toggle and, when open, the archived chats. */
export function ArchivedSection(props: ArchivedSectionProps) {
  const pid = () => props.list.pid();
  const open = () => !!M.S.archOpen[pid()] || (props.searching && props.chats.length > 0);
  return (
    <>
      <button
        type="button"
        onClick={() => {
          M.S.archOpen[pid()] = !M.S.archOpen[pid()];
        }}
        aria-expanded={open()}
        class="mt-2 flex h-8 w-full items-center gap-1.5 border-0 border-t border-border bg-transparent px-2 py-0 text-left text-small font-medium text-secondary"
      >
        <Icon name={open() ? "chevron-down" : "chevron-right"} size={CHEVRON_PX} />
        Archived
        <span class="font-normal">{props.chats.length}</span>
      </button>
      <Show when={open()}>
        <Show when={props.chats.length === 0}>
          <p class="mx-2.5 my-1 text-small text-muted">No archived chats.</p>
        </Show>
        <For each={props.chats}>{(chat) => <ArchivedChatRow chat={chat} list={props.list} />}</For>
      </Show>
    </>
  );
}
