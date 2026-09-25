import { createEffect, createMemo, onCleanup, Show } from "solid-js";
import { M } from "~/mock";
import { TimelineGrid } from "./TimelineGrid";
import { TimelinePhoneList } from "./TimelinePhoneList";
import { timelineCards } from "./timeline-model";

const NAV_OWNER = "timeline";

/**
 * The project's cards on a 32 day Gantt grid (phone: a day-by-day list). Port of
 * design/TimelineView.dc.html. Fills its parent and scrolls.
 */
export function TimelineView() {
  const cards = createMemo(() => timelineCards(M.filtered(M.S.route.pid ?? "")));

  // Arrow keys in the shell move the focus through these rows, top to bottom.
  createEffect(() => {
    M.nav = { owner: NAV_OWNER, rows: cards().map((c) => c.id) };
  });
  onCleanup(() => {
    if (M.nav?.owner === NAV_OWNER) M.nav = null;
  });

  return (
    <div class="absolute inset-0 overflow-auto bg-surface">
      <Show when={M.mobile} fallback={<TimelineGrid cards={cards()} />}>
        <TimelinePhoneList cards={cards()} />
      </Show>
    </div>
  );
}
