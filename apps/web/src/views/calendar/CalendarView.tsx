import { Show } from "solid-js";
import { M } from "~/mock";
import { CalendarAgenda } from "./CalendarAgenda";
import { CalendarGrid } from "./CalendarGrid";
import { CalendarToolbar } from "./CalendarToolbar";

/**
 * Month and week calendar of the open project: schedule runs, Google Calendar events,
 * and due cards (phone: a day-by-day agenda). Port of design/CalendarView.dc.html.
 */
export function CalendarView() {
  return (
    <div class="absolute inset-0 flex flex-col bg-surface">
      <CalendarToolbar />
      <Show when={M.mobile} fallback={<CalendarGrid />}>
        <CalendarAgenda />
      </Show>
    </div>
  );
}
