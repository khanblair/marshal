import { createMemo, For, Show } from "solid-js";
import { M } from "~/mock";
import { CalendarItemButton } from "./CalendarItemButton";
import type { CalCell, CalMode } from "./calendar-dates";
import { itemsOnDay } from "./calendar-source";

/** A month day shows this many items until its "N more" button is pressed. */
const MONTH_ITEM_LIMIT = 4;
const UNLIMITED = 99;

export interface CalendarDayCellProps {
  cell: CalCell;
  mode: CalMode;
}

/** One day of the grid: its date, its items, and a "N more" button when some are hidden. */
export function CalendarDayCell(props: CalendarDayCellProps) {
  const items = createMemo(() => itemsOnDay(props.cell.t));
  const limit = () =>
    props.mode === "month" && M.S.calExpand !== props.cell.key ? MONTH_ITEM_LIMIT : UNLIMITED;
  const hidden = () => items().length - limit();
  return (
    <div
      class="p-1.5 border-r border-b border-border flex flex-col gap-0.75 min-w-0"
      classList={{
        "min-h-28": props.mode === "month",
        "min-h-105": props.mode === "week",
        "bg-surface": props.cell.inMonth,
        "bg-surface-sunken": !props.cell.inMonth,
      }}
    >
      <span
        title={props.cell.full}
        class="self-start min-w-5.5 h-5.5 py-0 px-1.25 rounded-full text-caption leading-5.5 text-center"
        classList={{
          "font-bold bg-ink text-on-ink": props.cell.today,
          "font-normal bg-transparent": !props.cell.today,
          "text-primary": !props.cell.today && props.cell.inMonth,
          "text-muted": !props.cell.today && !props.cell.inMonth,
        }}
      >
        {props.cell.date}
      </span>
      <For each={items().slice(0, limit())}>{(item) => <CalendarItemButton item={item} />}</For>
      <Show when={hidden() > 0}>
        <button
          type="button"
          onClick={() => M.set({ calExpand: props.cell.key })}
          class="self-start h-5 py-0 px-1 border-none rounded-xs bg-transparent text-caption text-secondary hover:bg-surface-hover"
        >
          {`${hidden()} more`}
        </button>
      </Show>
    </div>
  );
}
