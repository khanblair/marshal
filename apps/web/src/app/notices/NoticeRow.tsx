import { Index, Show } from "solid-js";
import { NoticeAction } from "./NoticeAction";
import type { NoticeRowModel } from "./notice-list";

/** One card listed inside a notice: its title and sub line open it, its actions sit beside. */
export function NoticeRow(props: { row: NoticeRowModel }) {
  return (
    <div class="flex flex-wrap items-center gap-2 py-2 px-3 border-b border-border">
      <button
        type="button"
        onClick={() => props.row.open()}
        class="flex-[1_1_160px] min-w-0 flex flex-col items-start border-none bg-transparent p-0 text-left hover:underline"
      >
        <span class="text-small leading-4.5 font-semibold max-w-full overflow-hidden text-ellipsis whitespace-nowrap">
          {props.row.title}
        </span>
        <span class="flex flex-wrap gap-x-2 text-caption leading-4">
          <span class="text-secondary">{props.row.project}</span>
          <Show when={props.row.sub}>
            <span
              class={
                props.row.subTone === "needs" ? "text-status-needs-you-text" : "text-secondary"
              }
            >
              {props.row.sub}
            </span>
          </Show>
        </span>
      </button>
      <div class="flex flex-wrap gap-1">
        <Index each={props.row.actions}>
          {(action) => <NoticeAction action={action()} size="row" />}
        </Index>
      </div>
    </div>
  );
}
