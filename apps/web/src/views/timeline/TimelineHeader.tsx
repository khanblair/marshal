import { cx } from "@marshal/ui";
import { createMemo, For } from "solid-js";
import { M } from "~/mock";
import { type DayCell, dayCells, px, rangeLabel, TRACK_WIDTH_PX } from "./timeline-geometry";

function dayText(d: DayCell): string {
  if (d.today) return "text-primary font-bold";
  return d.weekend ? "text-muted font-normal" : "text-secondary font-normal";
}

/** Sticky header: the date range over the card column, then one cell per day. */
export function TimelineHeader() {
  // The cells follow today, which moves at midnight and when the time zone changes.
  const days = createMemo(() => dayCells(M.T0));
  return (
    <div class="sticky top-0 z-[12] flex h-13 bg-surface border-b border-border">
      <div class="sticky left-0 z-[2] w-[280px] flex-none flex items-end px-4 pt-0 pb-2 bg-surface border-r border-border font-semibold">
        {rangeLabel(M.T0)}
      </div>
      <div class="relative flex-none" style={{ width: px(TRACK_WIDTH_PX) }}>
        <For each={days()}>
          {(d) => (
            <div
              title={d.full}
              class={cx(
                "absolute top-0 bottom-0 w-10 flex flex-col items-center justify-end pb-1.5 gap-0 text-caption leading-4",
                dayText(d),
                d.weekend ? "bg-surface-sunken" : "bg-transparent",
              )}
              style={{ left: px(d.x) }}
            >
              <span>{d.weekday}</span>
              <span>{d.date}</span>
            </div>
          )}
        </For>
      </div>
    </div>
  );
}
