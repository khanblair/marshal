import { describe, expect, it } from "vitest";
import type { CalEvent, Card, Schedule } from "~/mock";
import { type CalSource, eventDialog, itemsForDay, scheduleDialog } from "./calendar-items";

const DAY_MS = 86_400_000;
const TODAY = new Date(2026, 8, 24).getTime();
const at = (offset: number) => new Date(2026, 8, 24 + offset).getTime();
const THURSDAY = 0;
const SATURDAY = 2;

const schedule = (over: Partial<Schedule>): Schedule => ({
  id: "s",
  name: "Job",
  kind: "job",
  icon: "clock",
  trigger: "Cron",
  when: "Every weekday at 9:00",
  time: "09:00",
  days: [1, 2, 3, 4, 5],
  action: "Do the thing",
  project: "All projects",
  enabled: true,
  missed: "Run once on wake",
  ...over,
});
const event = (over: Partial<CalEvent>): CalEvent => ({
  id: "e",
  title: "Meeting",
  time: "10:00",
  ...over,
});
const dueCard = (id: number, due: number | null): Card =>
  ({ id, title: `Card ${id}`, due }) as Card;

const source = (over: Partial<CalSource>): CalSource => ({
  schedules: [],
  events: [],
  cards: [],
  projectName: "api-gateway",
  today: TODAY,
  dayMs: DAY_MS,
  ...over,
});

describe("itemsForDay: schedules", () => {
  it("runs a cron schedule on its weekdays only", () => {
    const src = source({
      schedules: [schedule({ name: "Brief", kind: "brief", icon: "sunrise" })],
    });
    expect(itemsForDay(at(THURSDAY), src)).toMatchObject([
      { kind: "brief", icon: "sunrise", time: "09:00", label: "Brief" },
    ]);
    expect(itemsForDay(at(SATURDAY), src)).toEqual([]);
  });

  it("describes the schedule in the tooltip", () => {
    const [item] = itemsForDay(at(THURSDAY), source({ schedules: [schedule({ name: "Brief" })] }));
    expect(item?.tip).toBe("Brief. Every weekday at 9:00. Do the thing");
  });

  it("skips disabled schedules and other projects, but keeps All projects and its own", () => {
    const src = source({
      schedules: [
        schedule({ id: "off", enabled: false }),
        schedule({ id: "other", project: "web-dashboard" }),
        schedule({ id: "mine", project: "api-gateway" }),
        schedule({ id: "all", project: "All projects" }),
      ],
    });
    const ids = itemsForDay(at(THURSDAY), src).map(
      (i) => i.target.kind === "schedule" && i.target.schedule.id,
    );
    expect(ids).toEqual(["mine", "all"]);
  });

  it("runs an interval schedule on today and the two days after, every 30 minutes", () => {
    const src = source({
      schedules: [schedule({ name: "Loop", trigger: "Interval", time: "", days: [] })],
    });
    expect(itemsForDay(at(0), src)[0]?.label).toBe("Loop, every 30 min");
    expect(itemsForDay(at(2), src)).toHaveLength(1);
    expect(itemsForDay(at(3), src)).toHaveLength(0);
    expect(itemsForDay(at(-1), src)).toHaveLength(0);
  });
});

describe("itemsForDay: events and due cards", () => {
  it("shows an event on its weekdays or its day offset", () => {
    const src = source({
      events: [
        event({ id: "weekly", title: "Standup", days: [4] }),
        event({ id: "once", title: "Review", dayOffset: 1 }),
      ],
    });
    expect(itemsForDay(at(0), src).map((i) => i.label)).toEqual(["Standup"]);
    expect(itemsForDay(at(1), src).map((i) => i.label)).toEqual(["Review"]);
    expect(itemsForDay(at(2), src)).toEqual([]);
  });

  it("describes an event as coming from Google Calendar", () => {
    const [item] = itemsForDay(
      at(1),
      source({ events: [event({ title: "Review", dayOffset: 1 })] }),
    );
    expect(item).toMatchObject({
      kind: "event",
      icon: "calendar",
      tip: "Review at 10:00, from Google Calendar",
    });
    expect(item?.target).toEqual({ kind: "event", event: expect.anything(), day: at(1) });
  });

  it("shows a card on its due day, without a time", () => {
    const src = source({ cards: [dueCard(7, 2), dueCard(8, null), dueCard(9, 3)] });
    const items = itemsForDay(at(2), src);
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      kind: "due",
      icon: "calendar-check",
      time: "",
      label: "#7 Card 7",
      tip: "#7 Card 7 is due",
    });
  });

  it("finds a card due today by day offset 0", () => {
    expect(itemsForDay(at(0), source({ cards: [dueCard(7, 0)] }))).toHaveLength(1);
  });
});

describe("itemsForDay: order", () => {
  it("sorts by time and puts due cards last", () => {
    const src = source({
      schedules: [
        schedule({ name: "Late", time: "18:00" }),
        schedule({ name: "Early", time: "08:00" }),
      ],
      events: [event({ title: "Middle", time: "12:00", days: [4] })],
      cards: [dueCard(7, 0)],
    });
    expect(itemsForDay(at(0), src).map((i) => i.label)).toEqual([
      "Early",
      "Middle",
      "Late",
      "#7 Card 7",
    ]);
  });
});

describe("dialog text", () => {
  it("describes a schedule and how it treats missed runs", () => {
    expect(scheduleDialog(schedule({ name: "Brief", missed: "Run once on wake" }))).toEqual({
      title: "Brief",
      message: "Every weekday at 9:00. Do the thing. Missed runs: run once on wake.",
      action: "Edit schedule",
    });
  });

  it("describes an event with its full date", () => {
    const text = eventDialog(event({ title: "Review", time: "14:00" }), at(1));
    expect(text.title).toBe("Review");
    expect(text.action).toBe("Close");
    expect(text.message).toBe(
      `14:00 on ${new Date(at(1)).toLocaleDateString(undefined, { dateStyle: "full" })}. Synced from Google Calendar. Briefs use it for smart timing.`,
    );
  });
});
