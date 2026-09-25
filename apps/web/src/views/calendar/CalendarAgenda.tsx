import { DayHeading, Icon } from "@marshal/ui";
import { createMemo, For, Show } from "solid-js";
import { M } from "~/mock";
import { openCalItem } from "./calendar-actions";
import { agendaLabel, monthDays } from "./calendar-dates";
import type { CalItem } from "./calendar-items";
import { itemIconColor, itemsOnDay } from "./calendar-source";

const ICON_PX = 16;

function AgendaItem(props: { item: CalItem }) {
  return (
    <button
      type="button"
      onClick={() => openCalItem(props.item)}
      class="w-full flex items-center gap-2.5 min-h-11 p-0 border-0 border-b border-border bg-transparent text-left"
    >
      <span class="w-12 flex-none text-small text-secondary tabular-nums">
        {props.item.time || "Due"}
      </span>
      <span class="inline-flex" style={{ color: itemIconColor(props.item) }}>
        <Icon name={props.item.icon} size={ICON_PX} />
      </span>
      <span class="flex-1 min-w-0">{props.item.label}</span>
    </button>
  );
}

/** Phone layout: the month's days that have something on them, one section each. */
export function CalendarAgenda() {
  const days = createMemo(() =>
    monthDays(M.S.calCursor)
      .map((t) => ({ t, items: itemsOnDay(t) }))
      .filter((d) => d.items.length > 0),
  );
  return (
    <div class="flex-1 min-h-0 overflow-auto pt-2 px-4 pb-8">
      <Show when={days().length === 0}>
        <p class="text-secondary">Nothing is scheduled this month.</p>
      </Show>
      <For each={days()}>
        {(day) => (
          <section class="mb-2">
            <DayHeading today={day.t === M.T0} class="pt-2.5 pb-1.5">
              {agendaLabel(day.t, M.T0)}
            </DayHeading>
            <For each={day.items}>{(item) => <AgendaItem item={item} />}</For>
          </section>
        )}
      </For>
    </div>
  );
}
