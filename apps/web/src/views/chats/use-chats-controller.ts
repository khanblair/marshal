import { createMemo, createRenderEffect, createSignal } from "solid-js";
import type { Chat } from "~/mock";
import { M } from "~/mock";
import { threadSignature } from "./chat-model";
import { type ChatListState, useChatListState } from "./use-chat-list-state";
import { type ChatScroll, useChatScroll } from "./use-chat-scroll";

/** After a new chat starts, the design waits this long before focusing its message box. */
const FOCUS_DELAY_MS = 50;

export interface ChatsController {
  pid: () => string;
  /** The chat open in the pane, if any. */
  chat: () => Chat | undefined;
  list: ChatListState;
  scroll: ChatScroll;
  draft: () => string;
  setDraft: (value: string) => void;
  /** Sends `text`, or the draft when there is none. Nothing happens for blank text or no open chat. */
  send: (text?: string) => void;
  /** Gets the message box. Call it from a `ref`; it is set again each time the pane mounts. */
  setTextarea: (el: HTMLTextAreaElement) => void;
  focusComposer: () => void;
}

/**
 * The state the design keeps on its component: the draft (kept when the phone goes back to the
 * list), the scroll position, and the list's menu and rename. It also opens the first chat the
 * first time a project's chats are shown.
 */
export function useChatsController(): ChatsController {
  const pid = () => M.S.route.pid ?? "";
  createRenderEffect(() => {
    if (M.S.chatOpen[pid()] !== undefined) return;
    const first = M.chatsOf(pid()).find((chat) => !chat.archived);
    M.openChat(pid(), M.mobile ? null : (first?.id ?? null));
  });
  const chat = createMemo(() => M.chatById(pid(), M.S.chatOpen[pid()]));
  const scroll = useChatScroll(() => threadSignature(chat()));
  const [draft, setDraft] = createSignal("");
  let textarea: HTMLTextAreaElement | undefined;

  const send = (text?: string) => {
    const id = M.S.chatOpen[pid()];
    const body = text ?? draft();
    if (!body.trim() || !id) return;
    M.chatSend(pid(), id, body);
    setDraft("");
    scroll.afterSend();
  };
  return {
    pid,
    chat,
    list: useChatListState(pid, scroll.leaveAway),
    scroll,
    draft,
    setDraft,
    send,
    setTextarea: (el) => {
      textarea = el;
    },
    focusComposer: () => {
      setTimeout(() => textarea?.focus(), FOCUS_DELAY_MS);
    },
  };
}
