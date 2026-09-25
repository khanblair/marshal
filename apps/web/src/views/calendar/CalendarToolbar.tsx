import { Button, IconButton, IconLabel, SegmentedControl } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M } from "~/mock";
import { type CalMode, shiftCursor, titleLabel } from "./calendar-dates";

const MODE_OPTIONS = [
  { value: "month", label: "Month" },
  { value: "week", label: "Week" },
] as const;

const LEGEND = [
  { icon: "clock", label: "Scheduled job" },
  { icon: "sunrise", label: "Brief" },
  { icon: "calendar-check", label: "Due card" },
  { icon: "calendar", label: "Calendar event" },
] as const;

const LEGEND_ICON_PX = 12;

function shift(dir: 1 | -1): void {
  M.set({ calCursor: shiftCursor(M.S.calMode, M.S.calCursor, dir), calExpand: null });
}

/** Layout tabs, previous and next, Today, the month or week heading, and the legend. */
export function CalendarToolbar() {
  const unit = () => (M.S.calMode === "month" ? "month" : "week");
  return (
    <div class="flex-none flex flex-wrap items-center gap-x-3 gap-y-2 py-2 px-4 border-b border-border">
      <Show when={!M.mobile}>
        <SegmentedControl<CalMode>
          kind="tabs"
          label="Calendar layout"
          options={MODE_OPTIONS}
          value={M.S.calMode}
          onValueChange={(mode) => M.set({ calMode: mode })}
        />
      </Show>
      <div class="flex gap-0.5">
        <IconButton
          variant="outline"
          icon="chevron-left"
          label={`Previous ${unit()}`}
          onClick={() => shift(-1)}
        />
        <IconButton
          variant="outline"
          icon="chevron-right"
          label={`Next ${unit()}`}
          onClick={() => shift(1)}
        />
      </div>
      <Button size={28} onClick={() => M.set({ calCursor: M.T0 })}>
        Today
      </Button>
      <h2 class="m-0 text-subtitle leading-5.5 font-semibold">
        {titleLabel(M.S.calMode, M.S.calCursor, M.mobile)}
      </h2>
      <div class="flex-1" />
      <div class="flex flex-wrap gap-x-3.5 gap-y-1 text-caption leading-4 text-secondary">
        <For each={LEGEND}>
          {(l) => (
            <IconLabel icon={l.icon} size={LEGEND_ICON_PX}>
              {l.label}
            </IconLabel>
          )}
        </For>
      </div>
    </div>
  );
}
