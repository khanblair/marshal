import { createMemo, For } from "solid-js";
import { M } from "~/mock";
import { CalendarDayCell } from "./CalendarDayCell";
import { gridCells, weekdayLabels } from "./calendar-dates";

/** Weekday headings and the days of the month (six weeks) or of the week. */
export function CalendarGrid() {
  const cells = createMemo(() => gridCells(M.S.calMode, M.S.calCursor, M.T0));
  const headings = createMemo(() => weekdayLabels(M.S.calMode, M.S.calCursor, false));
  return (
    <div class="flex-1 min-h-0 overflow-auto pt-3 px-4 pb-4">
      <div class="grid grid-cols-7 border-l border-t border-border">
        <For each={headings()}>
          {(w) => (
            <div class="py-1.5 px-2 border-r border-b border-border bg-surface-sunken text-caption leading-4 font-semibold text-secondary">
              {w}
            </div>
          )}
        </For>
        <For each={cells()}>{(cell) => <CalendarDayCell cell={cell} mode={M.S.calMode} />}</For>
      </div>
    </div>
  );
}
