import { M } from "~/mock";
import { type CalItem, itemsForDay } from "./calendar-items";

/**
 * The items on one day of the open project. Reads the store, so call it inside a
 * memo or JSX and the caller updates when a schedule, event, or due date changes.
 */
export function itemsOnDay(t: number): CalItem[] {
  const pid = M.S.route.pid;
  return itemsForDay(t, {
    schedules: M.S.schedules,
    events: M.S.calEvents,
    cards: M.cardsOf(pid),
    projectName: M.proj(pid)?.name ?? "",
    today: M.T0,
    dayMs: M.D,
  });
}

/** Icon color of an item: a due card takes its state's color, the rest are secondary text. */
export function itemIconColor(item: CalItem): string {
  if (item.target.kind !== "card") return "var(--color-text-secondary)";
  return M.tone(M.STATUS[item.target.card.state].tone, "solid");
}
