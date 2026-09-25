import { Button, cx, EmptyState, NoResults } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M } from "~/mock";
import { BoardLane } from "./BoardLane";
import { boardPid, mobileColumn } from "./board-state";
import { ColumnHeaders } from "./ColumnHeaders";
import { ColumnTabs } from "./ColumnTabs";
import { createSwipeHandlers, stepColumn } from "./swipe";
import { useBoard } from "./use-board";

/** The text of the notice that filters or a search hide every card. */
function noMatchText(query: string | undefined): string {
  return query ? `No cards match "${query}".` : "No cards match these filters.";
}

/**
 * The project's cards as columns (Backlog to Done), optionally split into swimlanes. On phones it
 * shows one column at a time, with tabs and a swipe. Port of design/BoardView.dc.html. Fills its
 * parent. The shell moves the card focus with the arrow keys through the grid it publishes on
 * `M.nav`.
 */
export function BoardView() {
  const board = useBoard();
  const hasCards = () => M.cardsOf(boardPid()).length > 0;
  const swipe = createSwipeHandlers(
    () => M.mobile,
    (step) => M.set({ mobileCol: stepColumn(M.COLUMNS, mobileColumn(), step) }),
  );
  return (
    <div class="absolute inset-0 flex flex-col bg-canvas">
      <Show when={M.mobile}>
        <ColumnTabs counts={board.counts()} />
      </Show>
      <Show when={!hasCards()}>
        <EmptyState
          icon="square-kanban"
          action={
            <Button variant="primary" onClick={() => M.newCard()}>
              New card
            </Button>
          }
        >
          This board has no cards yet.
        </EmptyState>
      </Show>
      <Show when={hasCards() && board.list().length === 0}>
        <NoResults class="flex-none mx-4 mt-3 bg-surface" onClear={() => M.clearFilters()}>
          {noMatchText(M.S.query[boardPid()])}
        </NoResults>
      </Show>
      <Show when={hasCards()}>
        <div
          onTouchStart={swipe.onTouchStart}
          onTouchEnd={swipe.onTouchEnd}
          class={cx("flex-1 min-h-0 overflow-auto", M.mobile ? "p-3" : "px-4 pb-4")}
        >
          <div class={cx("inline-flex flex-col", M.mobile && "min-w-full")}>
            <Show when={!M.mobile}>
              <ColumnHeaders counts={board.counts()} />
            </Show>
            <For each={board.laneKeys()}>{(key) => <BoardLane lane={board.laneOf(key)} />}</For>
          </div>
        </div>
      </Show>
    </div>
  );
}
