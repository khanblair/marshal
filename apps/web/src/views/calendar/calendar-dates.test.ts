import { describe, expect, it } from "vitest";
import {
  agendaLabel,
  dayAfter,
  fullDate,
  gridCells,
  gridRange,
  mondayOf,
  monthDays,
  monthLabel,
  shiftCursor,
  titleLabel,
  weekdayLabels,
} from "./calendar-dates";

const local = (y: number, m: number, d: number, h = 0) => new Date(y, m, d, h).getTime();
/** Thursday, September 24, 2026. */
const THURSDAY = local(2026, 8, 24, 15);
const long = (t: number, options: Intl.DateTimeFormatOptions) =>
  new Date(t).toLocaleDateString(undefined, options);

describe("mondayOf", () => {
  it("goes back to midnight of the week's Monday", () => {
    expect(mondayOf(THURSDAY)).toBe(local(2026, 8, 21));
  });

  it("keeps a Monday and treats Sunday as the end of the week", () => {
    expect(mondayOf(local(2026, 8, 21, 9))).toBe(local(2026, 8, 21));
    expect(mondayOf(local(2026, 8, 27, 23))).toBe(local(2026, 8, 21));
  });

  it("crosses a month boundary", () => {
    expect(mondayOf(local(2026, 9, 1))).toBe(local(2026, 8, 28));
  });
});

describe("gridRange", () => {
  it("covers six weeks from the Monday before the 1st in month mode", () => {
    expect(gridRange("month", THURSDAY)).toEqual({ start: local(2026, 7, 31), count: 42 });
  });

  it("covers the cursor's week in week mode", () => {
    expect(gridRange("week", THURSDAY)).toEqual({ start: local(2026, 8, 21), count: 7 });
  });
});

describe("dayAfter", () => {
  it("adds calendar days, so a clock change never skips one", () => {
    expect(dayAfter(local(2026, 2, 28), 2).getDate()).toBe(30);
    expect(dayAfter(local(2026, 9, 31), 1).getMonth()).toBe(10);
  });
});

describe("gridCells", () => {
  const today = local(2026, 8, 24);
  const cells = gridCells("month", THURSDAY, today);

  it("has 42 consecutive days starting on a Monday", () => {
    expect(cells).toHaveLength(42);
    expect(new Date(cells[0]?.t ?? 0).getDay()).toBe(1);
    expect(cells[0]?.date).toBe(31);
    expect(cells[1]?.date).toBe(1);
  });

  it("marks the days outside the cursor's month", () => {
    expect(cells[0]?.inMonth).toBe(false);
    expect(cells[1]?.inMonth).toBe(true);
    expect(cells.filter((c) => c.inMonth)).toHaveLength(30);
  });

  it("marks only today, and gives every day a key and a full date", () => {
    const marked = cells.filter((c) => c.today);
    expect(marked).toHaveLength(1);
    expect(marked[0]?.key).toBe(String(today));
    expect(marked[0]?.full).toBe(fullDate(today));
  });

  it("has seven days in week mode, all in the month", () => {
    const week = gridCells("week", THURSDAY, today);
    expect(week.map((c) => c.date)).toEqual([21, 22, 23, 24, 25, 26, 27]);
    expect(week.every((c) => c.inMonth)).toBe(true);
  });
});

describe("weekdayLabels", () => {
  it("names the seven columns, Monday first", () => {
    const labels = weekdayLabels("month", THURSDAY, false);
    expect(labels).toHaveLength(7);
    expect(labels[0]).toBe(long(local(2026, 8, 21), { weekday: "short" }));
    expect(labels[6]).toBe(long(local(2026, 8, 27), { weekday: "short" }));
  });

  it("adds the date in week mode", () => {
    const labels = weekdayLabels("week", THURSDAY, false);
    expect(labels[0]).toBe(`${long(local(2026, 8, 21), { weekday: "short" })} 21`);
    expect(labels[6]).toMatch(/ 27$/);
  });

  it("uses one letter when narrow", () => {
    expect(weekdayLabels("month", THURSDAY, true)[0]).toBe(
      long(local(2026, 8, 21), { weekday: "narrow" }),
    );
  });
});

describe("shiftCursor", () => {
  it("moves a month at a time in month mode", () => {
    expect(new Date(shiftCursor("month", THURSDAY, 1)).getMonth()).toBe(9);
    expect(new Date(shiftCursor("month", THURSDAY, -1)).getMonth()).toBe(7);
  });

  it("overflows a short month like the design's setMonth does", () => {
    const jan31 = local(2026, 0, 31);
    expect(shiftCursor("month", jan31, 1)).toBe(local(2026, 2, 3));
  });

  it("moves seven days at a time in week mode", () => {
    expect(shiftCursor("week", THURSDAY, 1)).toBe(local(2026, 9, 1, 15));
    expect(shiftCursor("week", THURSDAY, -1)).toBe(local(2026, 8, 17, 15));
  });
});

describe("labels", () => {
  it("names the month", () => {
    expect(monthLabel(THURSDAY)).toBe(long(THURSDAY, { month: "long", year: "numeric" }));
  });

  it("titles month mode with the month and week mode with the Monday", () => {
    expect(titleLabel("month", THURSDAY, false)).toBe(monthLabel(THURSDAY));
    expect(titleLabel("week", THURSDAY, false)).toBe(
      `Week of ${long(local(2026, 8, 21), { month: "long", day: "numeric" })}`,
    );
  });

  it("always titles a phone with the month", () => {
    expect(titleLabel("week", THURSDAY, true)).toBe(monthLabel(THURSDAY));
  });

  it("puts Today in front of today's agenda heading only", () => {
    const today = local(2026, 8, 24);
    const heading = long(today, { weekday: "long", month: "long", day: "numeric" });
    expect(agendaLabel(today, today)).toBe(`Today, ${heading}`);
    expect(agendaLabel(today, local(2026, 8, 25))).toBe(heading);
  });
});

describe("monthDays", () => {
  it("lists every day of the month at midnight", () => {
    const days = monthDays(THURSDAY);
    expect(days).toHaveLength(30);
    expect(days[0]).toBe(local(2026, 8, 1));
    expect(days[29]).toBe(local(2026, 8, 30));
  });

  it("knows the length of February", () => {
    expect(monthDays(local(2026, 1, 10))).toHaveLength(28);
    expect(monthDays(local(2028, 1, 10))).toHaveLength(29);
  });
});
