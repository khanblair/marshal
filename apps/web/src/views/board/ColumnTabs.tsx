import { cx, StatusIcon } from "@marshal/ui";
import { For } from "solid-js";
import { type Column, M } from "~/mock";
import { mobileColumn } from "./board-state";

export interface ColumnTabsProps {
  counts: Record<Column, number>;
}

const TAB_ICON_PX = 14;

/** Phone only: one tab per column, and the board shows the selected one. */
export function ColumnTabs(props: ColumnTabsProps) {
  return (
    <div
      role="tablist"
      aria-label="Columns"
      class="flex flex-none gap-1.5 overflow-x-auto px-3 py-2 border-b border-border bg-surface"
    >
      <For each={M.COLUMNS}>
        {(col) => {
          const selected = () => col === mobileColumn();
          return (
            <button
              type="button"
              role="tab"
              aria-selected={selected()}
              onClick={() => M.set({ mobileCol: col })}
              class={cx(
                "inline-flex flex-none items-center gap-1.5 h-11 px-3 rounded-sm border text-small font-semibold text-primary",
                selected() ? "border-ink bg-surface-selected" : "border-border bg-surface",
              )}
            >
              <StatusIcon state={col} palette="column" size={TAB_ICON_PX} />
              {M.STATUS[col].label}
              <span class="font-normal text-secondary">{props.counts[col]}</span>
            </button>
          );
        }}
      </For>
    </div>
  );
}
