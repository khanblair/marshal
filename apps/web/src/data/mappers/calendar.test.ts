import type { CalendarEvent, CalendarList } from "@marshal/protocol";
import { afterEach, describe, expect, it } from "vitest";
import { setZone, startOfDay } from "../zone";
import { toCalEvents } from "./calendar";

afterEach(() => setZone(""));

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

const read = (event: CalendarEvent, today = TODAY) => {
  const list = {
    events: [event],
    googleConnected: true,
    googleError: "",
    googleStale: false,
  } as CalendarList;
  const [first] = toCalEvents(list, today);
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

describe("toCalEvents: an all-day event is on its own dates in every zone", () => {
  // Google sends midnight UTC of the date, which is the evening before in the Americas.
  const holiday = wire({
    allDay: true,
    start: "2026-10-09T00:00:00.000Z",
    end: "2026-10-10T00:00:00.000Z",
    startDate: "2026-10-09",
    endDate: "2026-10-10",
  });
  const trip = wire({
    allDay: true,
    start: "2026-10-09T00:00:00.000Z",
    end: "2026-10-12T00:00:00.000Z",
    startDate: "2026-10-09",
    endDate: "2026-10-12",
  });

  // Noon on 5 October in each zone.
  const NOONS = [
    ["Africa/Kampala", "2026-10-05T09:00:00Z"],
    ["UTC", "2026-10-05T12:00:00Z"],
    ["America/Los_Angeles", "2026-10-05T19:00:00Z"],
    ["Pacific/Auckland", "2026-10-04T23:00:00Z"],
  ] as const;

  for (const [zone, noon] of NOONS) {
    it(`puts it on 9 October in ${zone}`, () => {
      setZone(zone);
      const today = startOfDay(Date.parse(noon), zone);
      expect(read(holiday, today)).toMatchObject({ allDay: true, time: "", dayOffset: 4 });
      expect(read(holiday, today)).not.toHaveProperty("lastDayOffset");
      expect(read(trip, today)).toMatchObject({ dayOffset: 4, lastDayOffset: 6 });
    });
  }

  it("still reads the moments when the daemon sent no dates", () => {
    const old = wire({ allDay: true, start: local(6), end: local(9) });
    expect(read(old)).toMatchObject({ dayOffset: 1, lastDayOffset: 3 });
  });
});

describe("toCalEvents: a timed event is read in the chosen zone", () => {
  it("shows the clock and the day of the zone", () => {
    const standup = wire({ start: "2026-10-08T23:00:00.000Z", end: "2026-10-08T23:30:00.000Z" });
    // Noon on 4 October in each zone.
    for (const [zone, noon, time, offset] of [
      ["Africa/Kampala", "2026-10-04T09:00:00Z", "02:00", 5],
      ["America/Los_Angeles", "2026-10-04T19:00:00Z", "16:00", 4],
    ] as const) {
      setZone(zone);
      const today = startOfDay(Date.parse(noon), zone);
      expect(read(standup, today)).toMatchObject({
        time,
        endTime: time.replace(/:00$/, ":30"),
        dayOffset: offset,
      });
    }
  });
});
