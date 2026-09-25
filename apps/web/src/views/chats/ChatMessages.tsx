import { cx, JumpToLatest } from "@marshal/ui";
import { createMemo, Show } from "solid-js";
import { ChatThread } from "~/features/chat/ChatThread";
import type { Chat } from "~/mock";
import { M } from "~/mock";
import type { ChatScroll } from "./use-chat-scroll";

export interface ChatMessagesProps {
  chat: Chat;
  scroll: ChatScroll;
}

const JUMP_OFFSET_PX = 12;

/** The scrolling message thread, with Jump to latest floating over its bottom edge. */
export function ChatMessages(props: ChatMessagesProps) {
  const items = createMemo(() => M.decoMsgs(props.chat.msgs, null));
  return (
    <>
      <div
        ref={props.scroll.setEl}
        onScroll={props.scroll.onScroll}
        class="min-h-0 flex-1 overflow-auto"
      >
        <div class={cx("mx-auto max-w-[calc(72ch+48px)] pt-6 pb-8", M.mobile ? "px-4" : "px-6")}>
          <Show when={props.chat.msgs.length === 0}>
            <p class="m-0 mb-4 text-secondary">
              Say what you want in plain words. The first message names this chat.
            </p>
          </Show>
          <ChatThread items={items()} />
        </div>
      </div>
      <Show when={props.scroll.showJump()}>
        <JumpToLatest
          offset={JUMP_OFFSET_PX}
          onClick={props.scroll.jump}
          class="hover:bg-surface-raised!"
        />
      </Show>
    </>
  );
}
