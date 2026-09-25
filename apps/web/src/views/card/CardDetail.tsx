import { Show } from "solid-js";
import { M } from "~/mock";
import { CardPanel } from "./CardPanel";
import { createTerminalState } from "./terminal-state";

/**
 * The card detail panel: everything about the open card. It fills its parent and holds
 * no props. The panel under it is rebuilt when the open card changes, so drafts and open
 * menus reset with the card, as in the design; the terminal keys survive the change.
 */
export function CardDetail() {
  const terminal = createTerminalState();
  return (
    <div data-no-nav="1" class="absolute inset-0 flex flex-col bg-surface min-w-0">
      <Show when={M.card(M.S.openId)} keyed>
        {(card) => <CardPanel card={card} terminal={terminal} />}
      </Show>
    </div>
  );
}
