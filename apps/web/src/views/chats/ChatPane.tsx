import { Button, EmptyState } from "@marshal/ui";
import { Show } from "solid-js";
import { M } from "~/mock";
import { ChatFooter } from "./ChatFooter";
import { ChatHeader } from "./ChatHeader";
import { ChatMessages } from "./ChatMessages";
import type { ChatsController } from "./use-chats-controller";

export interface ChatPaneProps {
  ctl: ChatsController;
}

/** The open chat, or the empty state when none is open. */
export function ChatPane(props: ChatPaneProps) {
  return (
    <section aria-label={props.ctl.chat()?.title ?? "Chat"} class="flex min-w-0 flex-1 flex-col">
      <Show
        when={props.ctl.chat()}
        fallback={
          <EmptyState
            icon="messages-square"
            messageClass="max-w-[40ch]"
            action={
              <Button
                variant="primary"
                onClick={() => M.set({ newChatOpen: true })}
                class="hover:bg-ink!"
              >
                New chat
              </Button>
            }
          >
            Pick a chat, or start a new one with the Orchestrator, a role, or a card's agent.
          </EmptyState>
        }
      >
        {(chat) => (
          <>
            <ChatHeader chat={chat()} />
            <ChatMessages chat={chat()} scroll={props.ctl.scroll} />
            <ChatFooter chat={chat()} ctl={props.ctl} />
          </>
        )}
      </Show>
    </section>
  );
}
