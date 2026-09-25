import { Icon } from "@marshal/ui";
import { Show } from "solid-js";
import { openCalItem } from "./calendar-actions";
import type { CalItem } from "./calendar-items";
import { itemIconColor } from "./calendar-source";

const ICON_PX = 12;

/** One item in a day cell: icon, time, and a label cut off at the cell's edge. */
export function CalendarItemButton(props: { item: CalItem }) {
  const due = () => props.item.kind === "due";
  return (
    <button
      type="button"
      onClick={() => openCalItem(props.item)}
      title={props.item.tip}
      class="flex items-center gap-1 min-h-5.5 py-px px-1 border-none rounded-xs text-left text-caption leading-4 text-primary min-w-0 hover:bg-surface-selected"
      classList={{ "bg-surface-sunken": due(), "bg-transparent": !due() }}
    >
      <span class="inline-flex" style={{ color: itemIconColor(props.item) }}>
        <Icon name={props.item.icon} size={ICON_PX} />
      </span>
      <Show when={props.item.time}>
        <span class="flex-none text-secondary tabular-nums">{props.item.time}</span>
      </Show>
      <span class="flex-1 min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">
        {props.item.label}
      </span>
    </button>
  );
}
