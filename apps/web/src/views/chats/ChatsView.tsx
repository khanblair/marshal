import { Show } from "solid-js";
import { M } from "~/mock";
import { ChatList } from "./ChatList";
import { ChatPane } from "./ChatPane";
import { useChatsController } from "./use-chats-controller";

/**
 * A project's chats: the list of chats on the left and the open chat on the right. On a phone
 * the list and the chat take the whole screen in turn.
 */
export function ChatsView() {
  const ctl = useChatsController();
  const showList = () => !M.mobile || !M.S.chatOpen[ctl.pid()];
  const showChat = () => !M.mobile || !!M.S.chatOpen[ctl.pid()];
  return (
    <div data-no-nav="1" class="absolute inset-0 flex bg-surface">
      <Show when={showList()}>
        <ChatList list={ctl.list} onStarted={ctl.focusComposer} />
      </Show>
      <Show when={showChat()}>
        <ChatPane ctl={ctl} />
      </Show>
    </div>
  );
}
