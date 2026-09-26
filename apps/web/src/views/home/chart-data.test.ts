import type { DailyStats } from "~/mock/types";
import {
  barSummary,
  barTips,
  type ChartDays,
  costSeries,
  dateTicks,
  dollarLabel,
  finishedFromStats,
  finishedPerDay,
  seeded,
  tickIndexes,
} from "./chart-data";

const DAY = 86_400_000;
const T0 = new Date(2026, 8, 24).getTime();

/* The design's script, pasted with its inputs as parameters: the oracle. */
const rnd = (i: number) => {
  const x = Math.sin(i * 12.9898 + 78.233) * 43758.5453;
  return x - Math.floor(x);
};

function designFinished(N: number, mergedToday: number) {
  const dayIdx = (i: number) => Math.floor(T0 / DAY) - (N - 1 - i);
  return Array.from({ length: N }, (_, i) =>
    i === N - 1
      ? mergedToday
      : Math.round(
          1 + rnd(dayIdx(i)) * 5 + (new Date(dayIdx(i) * DAY).getDay() % 6 === 0 ? -1 : 1),
        ),
  );
}

function designSeries(N: number, ids: string[], today: Record<string, number>) {
  const dayIdx = (i: number) => Math.floor(T0 / DAY) - (N - 1 - i);
  const base: Record<string, number> = { api: 3.6, web: 2.8, mobile: 5.4 };
  const series = ids.map((id) =>
    Array.from({ length: N }, (_, i) =>
      i === N - 1
        ? (today[id] as number)
        : Math.max(0.2, (base[id] || 2) * (0.55 + rnd(dayIdx(i) * 3 + id.length) * 0.9)),
    ),
  );
  const total = Array.from({ length: N }, (_, i) =>
    series.reduce((a, s) => a + (s[i] as number), 0),
  );
  return { series, total };
}

const days = (range: number): ChartDays => ({ today: T0, dayMs: DAY, range });

describe("seeded", () => {
  it("is repeatable and stays in [0, 1)", () => {
    expect(seeded(20_000)).toBe(seeded(20_000));
    for (const seed of [0, 1, 19_000, 20_000, 123_456]) {
      expect(seeded(seed)).toBeGreaterThanOrEqual(0);
      expect(seeded(seed)).toBeLessThan(1);
    }
    expect(seeded(1)).not.toBe(seeded(2));
  });
});

describe("finishedPerDay", () => {
  for (const range of [7, 30, 90]) {
    it(`matches the design for ${range} days and ends on today's merges`, () => {
      const got = finishedPerDay(days(range), 4);
      expect(got).toEqual(designFinished(range, 4));
      expect(got).toHaveLength(range);
      expect(got.at(-1)).toBe(4);
    });
  }
});

describe("finishedFromStats", () => {
  const dailyStats = (days: number[]): DailyStats => ({
    range: 90,
    days: days.map((cardsFinished, i) => ({
      day: T0 - (days.length - 1 - i) * DAY,
      cardsFinished,
      merges: 0,
      ciFailures: 0,
      costMicros: 0,
    })),
    projects: [],
  });

  it("reads cardsFinished straight off the stored days, oldest first", () => {
    const stats = dailyStats([1, 2, 3, 4, 5, 6, 7]);
    expect(finishedFromStats(stats, 7)).toEqual([1, 2, 3, 4, 5, 6, 7]);
  });

  it("takes only the trailing range out of the widest stored series", () => {
    const stats = dailyStats(Array.from({ length: 90 }, (_, i) => i));
    expect(finishedFromStats(stats, 7)).toEqual([83, 84, 85, 86, 87, 88, 89]);
    expect(finishedFromStats(stats, 30)).toHaveLength(30);
  });
});

describe("costSeries", () => {
  const projects = [
    { id: "api", name: "Api", todayCost: 6.5 },
    { id: "web", name: "Web", todayCost: 4.25 },
    { id: "docs", name: "Docs", todayCost: 1.1 },
  ];
  for (const range of [7, 30, 90]) {
    it(`matches the design for ${range} days, with the total first`, () => {
      const want = designSeries(range, ["api", "web", "docs"], { api: 6.5, web: 4.25, docs: 1.1 });
      const got = costSeries(days(range), projects);
      expect(got.map((s) => s.name)).toEqual(["All projects", "Api", "Web", "Docs"]);
      expect(got[0]?.values).toEqual(want.total);
      expect(got.slice(1).map((s) => s.values)).toEqual(want.series);
      expect(got[1]?.values.at(-1)).toBe(6.5);
    });
  }

  it("never drops below twenty cents a day before today", () => {
    const got = costSeries(days(30), [{ id: "x", name: "X", todayCost: 0 }]);
    for (const value of got[1]?.values.slice(0, -1) ?? [])
      expect(value).toBeGreaterThanOrEqual(0.2);
  });

  it("has only the total when there are no projects", () => {
    const got = costSeries(days(7), []);
    expect(got).toEqual([{ name: "All projects", values: [0, 0, 0, 0, 0, 0, 0] }]);
  });
});

describe("date labels and tips", () => {
  it("picks three ticks for a week, four for a month or a quarter", () => {
    expect(tickIndexes(7)).toEqual([0, 3, 6]);
    expect(tickIndexes(30)).toEqual([0, 10, 20, 29]);
    expect(tickIndexes(90)).toEqual([0, 30, 60, 89]);
  });

  it("labels ticks with the day they stand for", () => {
    const short = (offset: number) =>
      new Date(T0 - offset * DAY).toLocaleDateString(undefined, { month: "short", day: "numeric" });
    expect(dateTicks(days(7))).toEqual([
      { index: 0, label: short(6) },
      { index: 3, label: short(3) },
      { index: 6, label: short(0) },
    ]);
  });

  it("words each bar as date, count, and cards", () => {
    const tips = barTips(days(7), [1, 2, 3, 4, 5, 6, 7]);
    expect(tips).toHaveLength(7);
    expect(tips[6]).toMatch(/: 7 cards$/);
    expect(tips[0]).toMatch(/: 1 cards$/);
  });

  it("summarises the bars and formats the cost axis", () => {
    expect(barSummary([1, 2, 3], 7)).toBe("6 cards finished in the last 7 days");
    expect(dollarLabel(11.6)).toBe("$12");
    expect(dollarLabel(0)).toBe("$0");
  });
});
