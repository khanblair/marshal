import { M } from "~/mock";

/* The "Coming up today" list: schedules, calendar events, and cards due today. */

export interface TodayItem {
  /** `09:30` for schedules and events, `Today` for due cards. */
  time: string;
  icon: string;
  iconColor: string;
  label: string;
  kind: "Brief" | "Job" | "Event" | "Due";
  open: () => void;
}

const QUIET = "var(--color-text-secondary)";

function scheduleItems(weekday: number): TodayItem[] {
  return M.S.schedules
    .filter((s) => s.enabled && s.days.includes(weekday) && s.time)
    .map((s) => ({
      time: s.time,
      icon: s.icon,
      iconColor: QUIET,
      label: s.name,
      kind: s.kind === "brief" ? "Brief" : "Job",
      open: () => {
        M.S.settingsSection = "schedules";
        M.S.schedEdit = s.id;
        M.go("settings");
      },
    }));
}

function eventItems(weekday: number): TodayItem[] {
  return M.S.calEvents
    .filter((e) => e.days?.includes(weekday) || e.dayOffset === 0)
    .map((e) => ({
      time: e.time,
      icon: "calendar",
      iconColor: QUIET,
      label: e.title,
      kind: "Event",
      open: () => M.toast(`${e.title} is from Google Calendar`),
    }));
}

function dueItems(): TodayItem[] {
  return M.S.cards
    .filter((c) => c.due === 0 && c.state !== "done")
    .map((c) => ({
      time: "Today",
      icon: "calendar-check",
      iconColor: M.tone(M.STATUS[c.state].tone, "solid"),
      label: `${M.cardLabelOf(c)} ${c.title} is due`,
      kind: "Due",
      open: () => M.openCard(c.id),
    }));
}

/** Everything happening on `weekday` (0 is Sunday), sorted by time. */
export function todayItems(weekday: number): TodayItem[] {
  return [...scheduleItems(weekday), ...eventItems(weekday), ...dueItems()].sort((a, b) =>
    a.time.localeCompare(b.time),
  );
}
