import { Index, Show } from "solid-js";
import type { MsgView } from "~/mock";
import { MessageRow } from "./MessageRow";

export interface ChatThreadProps {
  /** View models from `M.decoMsgs(list, cardId)`. */
  items: readonly MsgView[];
}

/**
 * A chat as a column of messages: user, agent and system text, tool calls, diff
 * summaries, plans, approvals, card previews, and card links. `M.decoMsgs` builds
 * fresh view models on every store change, so rows are keyed by position (`Index`)
 * and by message id: a row keeps its own state (an open plan editor, focus on an
 * approval) until its message really changes.
 */
export function ChatThread(props: ChatThreadProps) {
  return (
    <div class="flex flex-col gap-3 w-full">
      <Index each={props.items}>
        {(item) => (
          <Show when={item().id} keyed>
            {(_id) => <MessageRow item={item()} />}
          </Show>
        )}
      </Index>
    </div>
  );
}
