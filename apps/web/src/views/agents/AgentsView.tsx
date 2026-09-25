import { createMemo, createRenderEffect, onCleanup, Show } from "solid-js";
import { M } from "~/mock";
import { AgentsPhoneList } from "./AgentsPhoneList";
import { AgentsTable } from "./AgentsTable";
import { DESKTOP_MIN_WIDTH_PX, sortAgentCards, withSessions } from "./agents-model";

/**
 * Every agent session in the project, one row each, sorted by a header. The Orchestrator is
 * pinned first. Rows keep the keyboard focus when the store changes, because they are keyed on
 * the store's cards, not on copies.
 */
export function AgentsView() {
  const cards = createMemo(() =>
    sortAgentCards(withSessions(M.cardsOf(M.S.route.pid)), M.S.sort.agents),
  );
  const projectName = () => M.proj(M.S.route.pid)?.name ?? "";
  createRenderEffect(() => {
    M.nav = { owner: "agents", rows: cards().map((card) => card.id) };
  });
  onCleanup(() => {
    if (M.nav?.owner === "agents") M.nav = null;
  });
  return (
    <div class="absolute inset-0 overflow-auto bg-surface">
      <Show when={cards().length === 0}>
        <div class="px-6 py-12 text-center text-secondary">
          No sessions yet. Start a card to give it an agent session.
        </div>
      </Show>
      <Show
        when={M.mobile}
        fallback={
          <AgentsTable
            cards={cards()}
            projectName={projectName()}
            wide={M.S.vw >= DESKTOP_MIN_WIDTH_PX}
          />
        }
      >
        <AgentsPhoneList cards={cards()} projectName={projectName()} />
      </Show>
    </div>
  );
}
