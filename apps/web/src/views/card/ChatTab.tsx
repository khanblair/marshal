import { Composer, Icon, JumpToLatest, SegmentedControl, StatusDot } from "@marshal/ui";
import { Show } from "solid-js";
import { ChatThread } from "~/features/chat/ChatThread";
import { type Card, type CardView, M, type Mode } from "~/mock";
import { controlSize } from "./control-size";
import type { Panel } from "./panel-state";
import { TerminalView } from "./TerminalView";
import type { TerminalState } from "./terminal-state";
import { createAutoScroll } from "./use-auto-scroll";

export interface ChatTabProps {
  card: Card;
  c: CardView;
  panel: Panel;
  terminal: TerminalState;
}

const MODES = (switching: false | Mode) =>
  [
    { value: "chat", label: "Chat", icon: "message-square", disabled: !!switching },
    { value: "terminal", label: "Terminal", icon: "square-terminal", disabled: !!switching },
  ] as const;

/** What the bar says when the agent is not working on anything right now. */
function idleText(card: Card): string {
  if (card.state === "needs") return card.reason;
  if (card.asleep) return "Asleep. Sending a message wakes the session.";
  if (card.paused) return "Paused by you";
  if (card.state === "backlog") return "Not started";
  return card.state === "done" ? "Merged" : "Idle";
}

/** The bar above the chat: what the agent is doing, and the Chat or Terminal switch. */
function ModeBar(props: { card: Card; c: CardView }) {
  return (
    <div class="flex-none flex items-center gap-2 py-2 px-4 border-b border-border">
      <Show
        when={props.c.showDoing}
        fallback={<span class="flex-1 text-small text-secondary">{idleText(props.card)}</span>}
      >
        <span class="flex-1 min-w-0 flex items-center gap-2 text-small">
          <StatusDot state="working" />
          <span class="overflow-hidden text-ellipsis whitespace-nowrap">
            Doing now: {props.c.doing}
          </span>
        </span>
      </Show>
      <Show when={M.S.switching}>
        <span role="status" class="inline-flex items-center gap-1.5 text-small text-secondary">
          <Icon name="spinner" size={14} />
          {M.S.switching === "terminal" ? "Switching to terminal" : "Switching to chat"}
        </span>
      </Show>
      <SegmentedControl
        label="Chat or terminal"
        size={24}
        options={MODES(M.S.switching)}
        value={M.S.mode}
        onValueChange={(mode) => M.setMode(mode)}
      />
    </div>
  );
}

const SKELETON_WIDTHS = ["60%", "85%", "40%", "72%"] as const;

/** Placeholder lines shown for the moment the session switches between chat and terminal. */
function SwitchingBody() {
  return (
    <div
      class={`flex-1 flex flex-col gap-3 p-4 ${
        M.S.switching === "terminal" ? "bg-surface-sunken" : "bg-surface"
      }`}
    >
      {SKELETON_WIDTHS.map((width) => (
        <div class="h-3.5 rounded-xs bg-surface-selected" style={{ width }} />
      ))}
    </div>
  );
}

function ChatPane(props: ChatTabProps & { scroll: ReturnType<typeof createAutoScroll> }) {
  const draft = () => props.panel.state.draft;
  const send = () => {
    M.send(props.card.id, draft());
    props.panel.set({ draft: "", away: false });
  };
  const list = () => M.S.chat[props.card.id] ?? [];
  return (
    <>
      <div
        ref={props.scroll.chatRef}
        onScroll={props.scroll.onScroll}
        class="flex-1 min-h-0 overflow-auto p-4"
      >
        <Show when={list().length === 0}>
          <div class="py-8 px-2 text-center text-secondary">
            This card hasn't started. Start the card, or send a message to start its session.
          </div>
        </Show>
        <ChatThread items={M.decoMsgs(list(), props.card.id)} />
      </div>
      <Show when={props.panel.state.away && props.panel.state.unseen}>
        <JumpToLatest offset={10} onClick={props.scroll.jump} />
      </Show>
      <div class="flex-none pt-2.5 px-4 pb-3.5 border-t border-border">
        <Composer
          label="Message the agent"
          value={draft()}
          onValueChange={(value) => props.panel.set({ draft: value })}
          onSend={send}
          placeholder={
            props.card.asleep
              ? `Message ${props.c.num}. This wakes the session.`
              : `Message the ${props.card.agent} session on ${props.c.num}`
          }
          sendSize={controlSize()}
          maxHeight={140}
        />
        <span class="block mt-1.5 text-caption leading-4 text-muted">
          Enter sends. Shift and Enter adds a new line.
        </span>
      </div>
    </>
  );
}

/** The Chat tab: the mode bar, then the chat with its composer, the terminal, or the switching placeholder. */
export function ChatTab(props: ChatTabProps) {
  const scroll = createAutoScroll(props.card.id, props.panel);
  return (
    <>
      <ModeBar card={props.card} c={props.c} />
      <Show when={M.S.mode === "chat" && !M.S.switching}>
        <ChatPane {...props} scroll={scroll} />
      </Show>
      <Show when={M.S.mode === "terminal" && !M.S.switching}>
        <TerminalView card={props.card} terminal={props.terminal} scrollRef={scroll.termRef} />
      </Show>
      <Show when={M.S.switching}>
        <SwitchingBody />
      </Show>
    </>
  );
}
