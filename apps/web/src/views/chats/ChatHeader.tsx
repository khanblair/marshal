import { Button, IconButton } from "@marshal/ui";
import { For, Show } from "solid-js";
import type { Chat } from "~/mock";
import { M } from "~/mock";
import { targetBits } from "./chat-model";

export interface ChatHeaderProps {
  chat: Chat;
}

const BACK_ICON_PX = 20;

/** The open chat's title and target facts, a Back button on a phone, and Restore when archived. */
export function ChatHeader(props: ChatHeaderProps) {
  return (
    <div class="flex min-h-12 flex-none items-center gap-2 border-b border-border px-4 py-1">
      <Show when={M.mobile}>
        <IconButton
          label="Back to chats"
          icon="chevron-left"
          size={44}
          iconSize={BACK_ICON_PX}
          tone="default"
          class="-ml-3 rounded-none! hover:bg-transparent!"
          onClick={() => M.openChat(props.chat.pid, null)}
        />
      </Show>
      <div class="flex min-w-0 flex-1 flex-col">
        <span class="truncate font-semibold">{props.chat.title}</span>
        <span class="flex flex-wrap gap-x-3 gap-y-0 text-caption leading-4 text-secondary">
          <For each={targetBits(props.chat)}>{(bit) => <span>{bit}</span>}</For>
        </span>
      </div>
      <Show when={props.chat.archived}>
        <Button
          onClick={() => M.archiveChat(props.chat.pid, props.chat.id, false)}
          class="px-2.5! text-small hover:bg-surface!"
        >
          Restore
        </Button>
      </Show>
    </div>
  );
}
