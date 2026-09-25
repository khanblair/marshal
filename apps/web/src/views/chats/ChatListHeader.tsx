import { Button, Icon, Input } from "@marshal/ui";
import { Show } from "solid-js";
import { M } from "~/mock";
import { NewChatForm } from "./NewChatForm";

export interface ChatListHeaderProps {
  onStarted: () => void;
}

const ICON_PX = 14;

/** Search, the New chat button, and the New chat form under it. */
export function ChatListHeader(props: ChatListHeaderProps) {
  const pid = () => M.S.route.pid ?? "";
  return (
    <div class="flex flex-none flex-col gap-2 p-3">
      <div class="relative flex gap-2">
        <label class="relative flex min-w-0 flex-1 items-center">
          <span class="absolute left-2 inline-flex text-muted">
            <Icon name="search" size={ICON_PX} />
          </span>
          <Input
            value={M.S.chatQuery[pid()] || ""}
            onInput={(event) => {
              M.S.chatQuery[pid()] = event.currentTarget.value;
            }}
            placeholder="Search chats"
            aria-label="Search chats"
            class="w-full py-0 pr-2! pl-7! text-small"
          />
        </label>
        <Button
          variant="primary"
          icon="plus"
          iconSize={ICON_PX}
          aria-expanded={!!M.S.newChatOpen}
          onClick={() => M.set({ newChatOpen: !M.S.newChatOpen })}
          class="flex-none px-2.5! text-small hover:bg-ink!"
        >
          New chat
        </Button>
        <Show when={M.S.newChatOpen}>
          <NewChatForm onStarted={props.onStarted} />
        </Show>
      </div>
    </div>
  );
}
