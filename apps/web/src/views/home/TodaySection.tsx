import { Icon, ShowMoreFooter } from "@marshal/ui";
import { createMemo, Index, Show } from "solid-js";
import { M } from "~/mock";
import { openCalendar } from "./home-actions";
import { type TodayItem, todayItems } from "./today";
import { createShowAll } from "./use-show-all";

function TodayRow(props: { item: TodayItem }) {
  return (
    <button
      type="button"
      onClick={() => props.item.open()}
      class="flex items-center gap-2.5 py-2 px-0 border-x-0 border-b-0 border-t border-border bg-transparent text-left hover:bg-surface-hover"
    >
      <span class="w-11 flex-none tabular-nums text-secondary">{props.item.time}</span>
      <Icon name={props.item.icon} size={14} style={{ color: props.item.iconColor }} />
      <span class="flex-1 min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">
        {props.item.label}
      </span>
      <span class="text-caption text-muted">{props.item.kind}</span>
    </button>
  );
}

/** Home's "Coming up today": schedules, calendar events, and cards due today. */
export function TodaySection() {
  // Only the weekday matters, so the list does not rebuild on every clock tick.
  const weekday = createMemo(() => new Date(M.now()).getDay());
  const items = createMemo(() => todayItems(weekday()));
  const rows = createShowAll(items);
  return (
    <section aria-labelledby="h-today" class="flex flex-col">
      <h2 id="h-today" class="m-0 mb-2 text-subtitle leading-5.5 font-semibold">
        Coming up today
      </h2>
      <Show when={items().length === 0}>
        <p class="m-0 text-secondary">Nothing else is scheduled today.</p>
      </Show>
      <Index each={rows.visible()}>{(item) => <TodayRow item={item()} />}</Index>
      <ShowMoreFooter
        total={items().length}
        expanded={rows.expanded()}
        onToggle={rows.toggle}
        viewAllLabel="Open calendar"
        onViewAll={openCalendar}
      />
    </section>
  );
}
