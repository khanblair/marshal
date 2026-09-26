import { cx, ErrorState, JumpToLatest, SkeletonGroup, SkeletonLines } from "@marshal/ui";
import { createMemo, For, Show } from "solid-js";
import { ChatThread } from "~/features/chat/ChatThread";
import type { Chat } from "~/mock";
import { M } from "~/mock";
import type { ChatScroll } from "./use-chat-scroll";

export interface ChatMessagesProps {
  chat: Chat;
  scroll: ChatScroll;
}

const JUMP_OFFSET_PX = 12;

const SHORT_MESSAGE_LINES = 2;
const LONG_MESSAGE_LINES = 3;
/** Two messages' worth of lines, which is what a chat that is being read stands in for. */
const PLACEHOLDER_MESSAGES = [SHORT_MESSAGE_LINES, LONG_MESSAGE_LINES] as const;

/**
 * The scrolling message thread, with Jump to latest floating over its bottom edge. A chat the
 * daemon keeps has its history read when it opens, so until the messages are in the thread says why
 * it is empty (loading, or the daemon's own sentence and Try again), and only a chat that really has
 * no messages shows the invitation to say something (ui-rules.md 5.1, 5.2, and 5.6).
 */
export function ChatMessages(props: ChatMessagesProps) {
  const items = createMemo(() => M.decoMsgs(props.chat.msgs, null));
  const empty = () => props.chat.msgs.length === 0;
  return (
    <>
      <div
        ref={props.scroll.setEl}
        onScroll={props.scroll.onScroll}
        aria-busy={props.chat.history === "loading"}
        class="min-h-0 flex-1 overflow-auto"
      >
        <div class={cx("mx-auto max-w-[calc(72ch+48px)] pt-6 pb-8", M.mobile ? "px-4" : "px-6")}>
          <Show when={empty() && props.chat.history === "loading"}>
            <SkeletonGroup label="Loading messages" class="flex flex-col gap-5">
              <For each={PLACEHOLDER_MESSAGES}>{(lines) => <SkeletonLines lines={lines} />}</For>
            </SkeletonGroup>
          </Show>
          <Show when={empty() && props.chat.history === "failed"}>
            <ErrorState
              message={props.chat.historyError ?? "Marshal could not load this chat's messages."}
              onRetry={() => M.retryChat(props.chat.pid, props.chat.id)}
            />
          </Show>
          <Show when={empty() && !props.chat.history}>
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
