import { createMemo, createRoot } from "solid-js";
import { afterEach, describe, expect, it, vi } from "vitest";
import { currentTimeZone } from "./time-zones";
import {
  addDays,
  addMonths,
  clock,
  dateKey,
  dayNumber,
  dayOfWeek,
  daysBetween,
  daysBetweenDates,
  daysInMonth,
  formatDate,
  formatDateTime,
  formatShortDate,
  monthNumber,
  setZone,
  startOfDay,
  startOfMonth,
  yearNumber,
  zoneLabel,
  zoneName,
} from "./zone";

const KAMPALA = "Africa/Kampala";
const LA = "America/Los_Angeles";
const HOUR = 3_600_000;
const at = (iso: string): number => Date.parse(iso);

afterEach(() => setZone(""));

describe("startOfDay and dayOfWeek", () => {
  // 23:00 UTC on Thursday 8 October is already Friday 9 October in Kampala.
  const moment = at("2026-10-08T23:00:00Z");

  it("finds the day in Africa/Kampala", () => {
    expect(startOfDay(moment, KAMPALA)).toBe(at("2026-10-08T21:00:00Z"));
    expect(dayOfWeek(moment, KAMPALA)).toBe(5);
  });

  it("finds the day in UTC", () => {
    expect(startOfDay(moment, "UTC")).toBe(at("2026-10-08T00:00:00Z"));
    expect(dayOfWeek(moment, "UTC")).toBe(4);
  });

  it("finds the day in America/Los_Angeles", () => {
    expect(startOfDay(moment, LA)).toBe(at("2026-10-08T07:00:00Z"));
    expect(dayOfWeek(moment, LA)).toBe(4);
  });

  it("counts Sunday as 0", () => {
    expect(dayOfWeek(at("2026-10-11T12:00:00Z"), "UTC")).toBe(0);
  });

  it("is a fixed point on midnight and ends the day one millisecond before the next", () => {
    const midnight = startOfDay(moment, KAMPALA);
    expect(startOfDay(midnight, KAMPALA)).toBe(midnight);
    expect(dateKey(midnight - 1, KAMPALA)).toBe("2026-10-08");
    expect(dateKey(midnight, KAMPALA)).toBe("2026-10-09");
  });

  it("is right on a day that is 23 hours long", () => {
    const sunday = startOfDay(at("2026-03-08T20:00:00Z"), LA);
    const monday = startOfDay(at("2026-03-09T20:00:00Z"), LA);
    expect(sunday).toBe(at("2026-03-08T08:00:00Z"));
    expect(monday).toBe(sunday + 23 * HOUR);
  });

  it("is right on a day that is 25 hours long", () => {
    const sunday = startOfDay(at("2026-11-01T20:00:00Z"), LA);
    const monday = startOfDay(at("2026-11-02T20:00:00Z"), LA);
    expect(monday).toBe(sunday + 25 * HOUR);
  });

  it("starts a day whose midnight was skipped at the change", () => {
    // Havana moved its clocks at midnight on 8 March, so that day began at 01:00.
    const day = startOfDay(at("2026-03-08T12:00:00Z"), "America/Havana");
    expect(day).toBe(at("2026-03-08T05:00:00Z"));
    expect(dateKey(day, "America/Havana")).toBe("2026-03-08");
    expect(dateKey(day - 1, "America/Havana")).toBe("2026-03-07");
  });

  it("starts every day of a year at a moment whose day starts there", () => {
    const zones = [KAMPALA, "UTC", LA, "America/Havana", "Australia/Lord_Howe", "Asia/Kolkata"];
    for (const zone of zones) {
      for (let ms = at("2026-01-01T00:00:00Z"); ms < at("2027-01-01T00:00:00Z"); ms += 3 * HOUR) {
        const start = startOfDay(ms, zone);
        expect(start).toBeLessThanOrEqual(ms);
        expect(dateKey(start, zone)).toBe(dateKey(ms, zone));
        expect(dateKey(start - 1, zone)).not.toBe(dateKey(ms, zone));
      }
    }
  });
});

describe("the date in a zone", () => {
  const moment = at("2026-10-08T23:00:00Z");

  it("is October 9 in Kampala and October 8 in Los Angeles", () => {
    expect(dateKey(moment, KAMPALA)).toBe("2026-10-09");
    expect(dateKey(moment, LA)).toBe("2026-10-08");
    expect(dateKey(moment, "UTC")).toBe("2026-10-08");
  });

  it("reads the day, month, and year", () => {
    expect(dayNumber(moment, KAMPALA)).toBe(9);
    expect(monthNumber(moment, KAMPALA)).toBe(10);
    expect(yearNumber(moment, KAMPALA)).toBe(2026);
    expect(dayNumber(moment, LA)).toBe(8);
  });

  it("starts and measures a month", () => {
    expect(startOfMonth(moment, KAMPALA)).toBe(at("2026-09-30T21:00:00Z"));
    expect(daysInMonth(moment, KAMPALA)).toBe(31);
    expect(daysInMonth(at("2026-02-10T00:00:00Z"), "UTC")).toBe(28);
  });

  it("moves by months the way setMonth does", () => {
    expect(addMonths(at("2026-09-24T09:00:00Z"), 1, "UTC")).toBe(at("2026-10-24T09:00:00Z"));
    expect(addMonths(at("2026-01-24T09:00:00Z"), -1, "UTC")).toBe(at("2025-12-24T09:00:00Z"));
    expect(addMonths(at("2026-01-31T09:00:00Z"), 1, "UTC")).toBe(at("2026-03-03T09:00:00Z"));
  });
});

describe("daysBetween", () => {
  it("counts calendar days in the zone, not 24 hour spans", () => {
    const from = at("2026-10-08T23:00:00Z");
    const to = at("2026-10-10T01:00:00Z");
    expect(daysBetween(from, to, "UTC")).toBe(2);
    expect(daysBetween(from, to, KAMPALA)).toBe(1);
    expect(daysBetween(from, to, LA)).toBe(1);
    expect(daysBetween(to, from, "UTC")).toBe(-2);
  });

  it("counts a span with a clock change by its dates", () => {
    const from = startOfDay(at("2026-03-08T20:00:00Z"), LA);
    const to = startOfDay(at("2026-03-10T20:00:00Z"), LA);
    expect(to - from).toBe(47 * HOUR);
    expect(daysBetween(from, to, LA)).toBe(2);
  });

  it("counts the days between two dates that belong to no zone", () => {
    expect(daysBetweenDates("2026-10-08", "2026-10-09")).toBe(1);
    expect(daysBetweenDates("2026-10-30", "2026-11-02")).toBe(3);
    expect(daysBetweenDates("2026-10-09", "2026-10-08")).toBe(-1);
    expect(daysBetweenDates("", "2026-10-08")).toBeNull();
  });
});

describe("clock", () => {
  it("shows the time of day on a 24 hour clock", () => {
    const moment = at("2026-10-08T06:30:00Z");
    expect(clock(moment, KAMPALA)).toBe("09:30");
    expect(clock(moment, "UTC")).toBe("06:30");
    expect(clock(moment, LA)).toBe("23:30");
  });

  it("shows midnight as 00:00", () => {
    expect(clock(at("2026-10-08T21:00:00Z"), KAMPALA)).toBe("00:00");
  });
});

describe("addDays", () => {
  it("adds days in Kampala, which has no clock change", () => {
    const moment = at("2026-10-08T06:30:00Z");
    expect(addDays(moment, 3, KAMPALA)).toBe(moment + 72 * HOUR);
    expect(addDays(moment, 0, KAMPALA)).toBe(moment);
  });

  it("keeps the wall clock across the change into summer time", () => {
    const evening = at("2026-03-07T17:30:00Z");
    expect(clock(evening, LA)).toBe("09:30");
    const next = addDays(evening, 1, LA);
    expect(next).toBe(evening + 23 * HOUR);
    expect(clock(next, LA)).toBe("09:30");
  });

  it("keeps the wall clock across the change out of summer time", () => {
    const morning = at("2026-10-31T16:30:00Z");
    const next = addDays(morning, 2, LA);
    expect(next).toBe(morning + 49 * HOUR);
    expect(clock(next, LA)).toBe("09:30");
    expect(addDays(next, -2, LA)).toBe(morning);
  });

  it("steps from a midnight to the next midnight", () => {
    const sunday = startOfDay(at("2026-03-08T20:00:00Z"), LA);
    expect(addDays(sunday, 1, LA)).toBe(startOfDay(at("2026-03-09T20:00:00Z"), LA));
    expect(addDays(sunday, -1, LA)).toBe(startOfDay(at("2026-03-07T20:00:00Z"), LA));
  });
});

describe("the chosen zone", () => {
  const moment = at("2026-10-08T23:00:00Z");

  it("is the device's own zone until one is chosen", () => {
    expect(zoneName()).toBe(currentTimeZone());
    setZone(LA);
    expect(zoneName()).toBe(LA);
    setZone("");
    expect(zoneName()).toBe(currentTimeZone());
  });

  it("falls back to the device's zone for a name the browser does not know", () => {
    setZone("Mars/Olympus");
    expect(zoneName()).toBe(currentTimeZone());
  });

  it("is what the functions use when no zone is passed", () => {
    setZone(KAMPALA);
    expect(dateKey(moment)).toBe("2026-10-09");
    expect(startOfDay(moment)).toBe(at("2026-10-08T21:00:00Z"));
    setZone(LA);
    expect(dateKey(moment)).toBe("2026-10-08");
    expect(clock(moment)).toBe("16:00");
  });

  it("tells a memo that reads it when it changes", () => {
    createRoot((dispose) => {
      const day = createMemo(() => dateKey(moment));
      setZone(KAMPALA);
      expect(day()).toBe("2026-10-09");
      setZone(LA);
      expect(day()).toBe("2026-10-08");
      dispose();
    });
  });
});

describe("showing a date", () => {
  const moment = at("2026-10-08T23:00:00Z");

  it("shows the day of the zone", () => {
    expect(formatDate(moment, { day: "numeric" }, KAMPALA)).toBe("9");
    expect(formatDate(moment, { day: "numeric" }, LA)).toBe("8");
  });

  it("shows a short date, and a date with its time", () => {
    expect(formatShortDate(moment, KAMPALA)).toBe(
      new Date(moment).toLocaleDateString(undefined, {
        month: "short",
        day: "numeric",
        timeZone: KAMPALA,
      }),
    );
    expect(formatDateTime(moment, KAMPALA)).toBe(
      new Date(moment).toLocaleString(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
        timeZone: KAMPALA,
      }),
    );
  });

  it("does not throw for a time that is not a number", () => {
    expect(formatDate(Number.NaN, { day: "numeric" }, "UTC")).toBe("Invalid Date");
  });
});

describe("zoneLabel", () => {
  it("names UTC", () => {
    expect(zoneLabel(at("2026-10-08T12:00:00Z"), "UTC")).toBe("UTC");
  });

  it("follows summer time", () => {
    expect(zoneLabel(at("2026-07-01T12:00:00Z"), LA)).toMatch(/^(PDT|GMT-7)$/);
    expect(zoneLabel(at("2026-01-01T12:00:00Z"), LA)).toMatch(/^(PST|GMT-8)$/);
  });

  it("names the chosen zone when none is passed", () => {
    setZone("UTC");
    expect(zoneLabel()).toBe("UTC");
  });
});

describe("remembering the zone on this device", () => {
  const KEY = "marshal.zone";
  afterEach(() => {
    window.localStorage.removeItem(KEY);
    vi.resetModules();
  });

  it("keeps the chosen zone, and forgets it when none is chosen", () => {
    setZone("Africa/Kampala");
    expect(window.localStorage.getItem(KEY)).toBe("Africa/Kampala");
    setZone("");
    expect(window.localStorage.getItem(KEY)).toBeNull();
  });

  it("starts the next visit in the zone it remembered", async () => {
    window.localStorage.setItem(KEY, "America/Los_Angeles");
    vi.resetModules();
    const fresh = await import("./zone");
    expect(fresh.zoneName()).toBe("America/Los_Angeles");
  });

  it("ignores a remembered name the browser does not know", async () => {
    window.localStorage.setItem(KEY, "Not/AZone");
    vi.resetModules();
    const fresh = await import("./zone");
    expect(fresh.zoneName()).not.toBe("Not/AZone");
  });
});
