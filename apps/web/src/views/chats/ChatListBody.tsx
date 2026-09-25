import { createMemo, For, Show } from "solid-js";
import { M } from "~/mock";
import { ArchivedSection } from "./ArchivedSection";
import { ChatRow } from "./ChatRow";
import { matchesQuery, noChatsText } from "./chat-model";
import type { ChatListState } from "./use-chat-list-state";

export interface ChatListBodyProps {
  list: ChatListState;
}

/** The scrolling list: active chats, then the Archived section. */
export function ChatListBody(props: ChatListBodyProps) {
  const query = () => M.S.chatQuery[props.list.pid()] || "";
  const matches = createMemo(() =>
    M.chatsOf(props.list.pid()).filter((chat) => matchesQuery(chat, query())),
  );
  const active = createMemo(() => matches().filter((chat) => !chat.archived));
  const archived = createMemo(() => matches().filter((chat) => chat.archived));
  return (
    // biome-ignore lint/a11y/useSemanticElements: the design uses a div with a list role; a ul brings list padding and bullets, and Tailwind preflight is off
    <div role="list" class="min-h-0 flex-1 overflow-auto px-2 pb-3">
      <Show when={active().length === 0}>
        <p class="m-2 text-small text-secondary">{noChatsText(query())}</p>
      </Show>
      <For each={active()}>{(chat) => <ChatRow chat={chat} list={props.list} />}</For>
      <ArchivedSection chats={archived()} list={props.list} searching={!!query()} />
    </div>
  );
}
