import { batch } from "solid-js";
import { M } from "~/mock";
import { type CalItem, eventDialog, scheduleDialog } from "./calendar-items";

/** What a click on a calendar item does: a card opens, a schedule or event asks first. */
export function openCalItem(item: CalItem): void {
  const { target } = item;
  if (target.kind === "card") {
    M.openCard(target.card.id);
    return;
  }
  if (target.kind === "event") {
    M.confirm({ ...eventDialog(target.event, target.day), run: () => {} });
    return;
  }
  const { schedule } = target;
  M.confirm({
    ...scheduleDialog(schedule),
    run: () =>
      batch(() => {
        M.set({ settingsSection: "schedules", schedEdit: schedule.id });
        M.go("settings");
      }),
  });
}
