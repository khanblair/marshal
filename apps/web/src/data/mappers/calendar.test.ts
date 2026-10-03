import type { CalendarEvent, CalendarList } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { toCalEvents } from "./calendar";

const TODAY = new Date(2026, 9, 5).getTime();
const local = (day: number, hour = 0, minute = 0) =>
  new Date(2026, 9, day, hour, minute).toISOString();

const wire = (over: Partial<CalendarEvent>): CalendarEvent => ({
  id: "e",
  title: "Event",
  start: local(5, 9),
  end: local(5, 10),
  allDay: false,
  location: "",
  url: "",
  joinUrl: "",
  calendar: "",
  ...over,
});

const read = (event: CalendarEvent) => {
  const list = {
    events: [event],
    googleConnected: true,
    googleError: "",
    googleStale: false,
  } as CalendarList;
  const [first] = toCalEvents(list, TODAY);
  return first;
};

describe("toCalEvents: the days an event covers", () => {
  it("covers one day for an event that starts and ends on it", () => {
    expect(read(wire({}))).toMatchObject({ dayOffset: 0 });
    expect(read(wire({}))).not.toHaveProperty("lastDayOffset");
  });

  it("covers a one-day all-day event on that day only, since its end is the next midnight", () => {
    const one = read(wire({ allDay: true, start: local(6), end: local(7) }));
    expect(one).toMatchObject({ dayOffset: 1, allDay: true });
    expect(one).not.toHaveProperty("lastDayOffset");
  });

  it("covers every day of a multi-day all-day event, and stops before its exclusive end", () => {
    const trip = read(wire({ allDay: true, start: local(6), end: local(9) }));
    expect(trip).toMatchObject({ dayOffset: 1, lastDayOffset: 3 });
  });

  it("covers the next day for an event that runs overnight", () => {
    const deploy = read(wire({ start: local(5, 22), end: local(6, 2) }));
    expect(deploy).toMatchObject({ dayOffset: 0, lastDayOffset: 1 });
    expect(deploy?.endTime).toBeUndefined();
  });

  it("does not spill onto the next day when an event ends exactly at midnight", () => {
    expect(read(wire({ start: local(5, 22), end: local(6, 0) }))).not.toHaveProperty(
      "lastDayOffset",
    );
  });

  it("covers one day when Google gave no end", () => {
    expect(read(wire({ end: null }))).not.toHaveProperty("lastDayOffset");
  });
});
