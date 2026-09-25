import { Badge, StatusIcon } from "@marshal/ui";
import { For } from "solid-js";
import { type Column, M } from "~/mock";

export interface ColumnHeadersProps {
  counts: Record<Column, number>;
}

/** Desktop and tablet: the sticky row of column names and counts above the lanes. */
export function ColumnHeaders(props: ColumnHeadersProps) {
  return (
    <div class="sticky top-0 z-sticky flex gap-3 pt-3 pb-2 bg-canvas">
      <For each={M.COLUMNS}>
        {(col) => (
          <div class="flex w-72 flex-none items-center gap-2 px-2 h-7">
            <StatusIcon state={col} palette="column" size={16} />
            <h2 class="m-0 text-body leading-5 font-semibold">{M.STATUS[col].label}</h2>
            <Badge
              tone="selected"
              class="min-w-5 leading-5"
              aria-label={`${props.counts[col]} cards`}
            >
              {props.counts[col]}
            </Badge>
          </div>
        )}
      </For>
    </div>
  );
}
