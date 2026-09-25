import { Composer, cx } from "@marshal/ui";
import { For, Show } from "solid-js";
import type { Chat } from "~/mock";
import { M } from "~/mock";
import { composerPlaceholder, SUGGESTIONS } from "./chat-model";
import type { ChatsController } from "./use-chats-controller";

export interface ChatFooterProps {
  chat: Chat;
  ctl: ChatsController;
}

const CHIP_CLASS =
  "h-7 flex-none rounded-full border border-border bg-surface px-2.5 py-0 text-small text-secondary hover:bg-surface-hover hover:text-primary";

/** Quick sends, the message box, and the keyboard hint (not on a phone). */
export function ChatFooter(props: ChatFooterProps) {
  return (
    <div class={cx("flex-none border-t border-border pt-3 pb-3.5", M.mobile ? "px-4" : "px-6")}>
      <div class="mx-auto flex max-w-[calc(72ch+16px)] flex-col gap-2">
        <div class="flex gap-1.5 overflow-x-auto">
          <For each={SUGGESTIONS}>
            {(label) => (
              <button type="button" onClick={() => props.ctl.send(label)} class={CHIP_CLASS}>
                {label}
              </button>
            )}
          </For>
        </div>
        <Composer
          value={props.ctl.draft()}
          onValueChange={props.ctl.setDraft}
          onSend={() => props.ctl.send()}
          label="Message"
          placeholder={composerPlaceholder(props.chat)}
          textareaRef={props.ctl.setTextarea}
        />
        <Show when={!M.mobile}>
          <span class="text-caption leading-4 text-muted">
            Enter sends. Shift and Enter adds a new line. Cards created here appear on the{" "}
            {M.proj(props.chat.pid)?.name} board.
          </span>
        </Show>
      </div>
    </div>
  );
}
