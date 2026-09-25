import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { type HomeSnapshot, homeSnapshot, resetHome } from "./test-support";
import { todayItems } from "./today";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const snapshot: HomeSnapshot = homeSnapshot();
const THURSDAY = 4;

beforeEach(() => {
  vi.useFakeTimers();
  resetHome(snapshot);
  M.S.calEvents = [];
  for (const card of M.S.cards) card.due = null;
});
afterEach(() => {
  vi.useRealTimers();
});

const schedule = (patch: Record<string, unknown>) => {
  const [first] = snapshot.schedules;
  if (!first) throw new Error("seed has no schedules");
  return { ...first, enabled: true, ...patch } as (typeof M.S.schedules)[number];
};

describe("todayItems", () => {
  it("lists the enabled schedules of the weekday with their time and kind", () => {
    M.S.schedules = [
      schedule({ id: "a", name: "Brief", kind: "brief", time: "08:00", days: [THURSDAY] }),
      schedule({ id: "b", name: "Job", kind: "job", time: "09:00", days: [THURSDAY] }),
      schedule({ id: "c", name: "Monday only", time: "10:00", days: [1] }),
      schedule({ id: "d", name: "Disabled", time: "11:00", days: [THURSDAY], enabled: false }),
      schedule({ id: "e", name: "No time", time: "", days: [THURSDAY] }),
    ];
    const items = todayItems(THURSDAY);
    expect(items.map((i) => [i.time, i.label, i.kind])).toEqual([
      ["08:00", "Brief", "Brief"],
      ["09:00", "Job", "Job"],
    ]);
  });

  it("includes events of the weekday and events for today", () => {
    M.S.calEvents = [
      { id: "1", title: "Every Thursday", time: "12:00", days: [THURSDAY] },
      { id: "2", title: "Once today", time: "13:00", dayOffset: 0 },
      { id: "3", title: "Tomorrow", time: "14:00", dayOffset: 1 },
      { id: "4", title: "Other day", time: "15:00", days: [1] },
    ];
    M.S.schedules = [];
    expect(todayItems(THURSDAY).map((i) => i.label)).toEqual(["Every Thursday", "Once today"]);
  });

  it("includes open cards due today, colored by their state", () => {
    M.S.schedules = [];
    const working = M.S.cards.find((c) => c.state === "working");
    const done = M.S.cards.find((c) => c.state === "done");
    if (!working || !done) throw new Error("seed lacks a working or done card");
    working.due = 0;
    done.due = 0;
    const items = todayItems(THURSDAY);
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      time: "Today",
      icon: "calendar-check",
      kind: "Due",
      label: `#${working.id} ${working.title} is due`,
      iconColor: "var(--color-status-working-solid)",
    });
  });

  it("sorts by time, with Today after clock times", () => {
    M.S.schedules = [schedule({ id: "late", name: "Late", time: "18:00", days: [THURSDAY] })];
    M.S.calEvents = [{ id: "1", title: "Early", time: "07:30", days: [THURSDAY] }];
    const due = M.S.cards.find((c) => c.state === "review");
    if (!due) throw new Error("seed has no review card");
    due.due = 0;
    expect(todayItems(THURSDAY).map((i) => i.time)).toEqual(["07:30", "18:00", "Today"]);
  });

  it("opens a schedule in settings, an event as a toast, and a due card in the panel", () => {
    M.S.schedules = [schedule({ id: "open-me", time: "08:00", days: [THURSDAY] })];
    M.S.calEvents = [{ id: "1", title: "Standup", time: "09:00", days: [THURSDAY] }];
    const due = M.S.cards.find((c) => c.state === "review");
    if (!due) throw new Error("seed has no review card");
    due.due = 0;
    const [job, event, card] = todayItems(THURSDAY);
    job?.open();
    expect(M.S.route.page).toBe("settings");
    expect(M.S.settingsSection).toBe("schedules");
    expect(M.S.schedEdit).toBe("open-me");
    event?.open();
    expect(M.S.toasts.at(-1)?.msg).toBe("Standup is from Google Calendar");
    card?.open();
    expect(M.S.openId).toBe(due.id);
  });
});
