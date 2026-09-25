import { Button, cx } from "@marshal/ui";
import { createMemo, For, Show } from "solid-js";
import { type Card, M } from "~/mock";
import { AddCardRow } from "./AddCardRow";
import { type ColumnModel, canQuickAdd, columnMinHeightClass, emptyText } from "./board-model";
import { boardPid, boardSwim } from "./board-state";
import { CardItem } from "./CardItem";
import { QuickAddForm } from "./QuickAddForm";

export interface BoardColumnProps {
  laneKey: string;
  column: ColumnModel;
}

/** Wraps one card so its view model updates in place while the store changes. */
function BoardCard(props: { card: Card }) {
  const view = createMemo(() => M.deco(props.card));
  return <CardItem c={view()} />;
}

/**
 * One column of one lane: its cards, the add card controls (Backlog, Planning, Working), the
 * Done limit, and an empty message. The border lights up while a dragged card is over it.
 */
export function BoardColumn(props: BoardColumnProps) {
  const adding = () =>
    canQuickAdd(props.column.col) && M.S.quickAddAt === `${props.laneKey}:${props.column.col}`;
  return (
    <section
      data-col={props.column.col}
      aria-label={`${M.STATUS[props.column.col].label}, ${props.column.total} cards`}
      class={cx(
        "flex flex-none flex-col gap-2 p-2 rounded-lg bg-surface-sunken border transition-[border-color] duration-fast ease-standard",
        M.mobile ? "w-full" : "w-72",
        columnMinHeightClass(boardSwim(), M.mobile),
        M.S.dropCol === props.column.col ? "border-border-strong" : "border-transparent",
      )}
    >
      <For each={props.column.cards}>{(card) => <BoardCard card={card} />}</For>
      <Show when={adding()}>
        <QuickAddForm laneKey={props.laneKey} col={props.column.col} />
      </Show>
      <Show when={canQuickAdd(props.column.col) && !adding()}>
        <AddCardRow laneKey={props.laneKey} col={props.column.col} />
      </Show>
      <Show when={props.column.showAll}>
        <Button
          variant="quiet"
          size={28}
          onClick={() => {
            M.S.showAllDone[boardPid()] = true;
          }}
        >
          {`Show all ${props.column.total}`}
        </Button>
      </Show>
      <Show when={props.column.total === 0}>
        <div class="py-3 px-2 text-small leading-4.5 text-muted text-center">
          {emptyText(!!M.S.dragId, boardSwim())}
        </div>
      </Show>
    </section>
  );
}
