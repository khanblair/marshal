import { describe, expect, it } from "vitest";
import { cardKey } from "~/mock/card-key";
import {
  barWidth,
  barX,
  DAY_COUNT,
  dayCells,
  dependencyLines,
  depLine,
  px,
  rangeLabel,
  snapDays,
  spanOf,
  svgHeight,
  TODAY_LINE_X_PX,
  TOTAL_WIDTH_PX,
  TRACK_WIDTH_PX,
} from "./timeline-geometry";

const DAY_MS = 86_400_000;
/** A Thursday, local midnight. */
const TODAY = new Date(2026, 8, 24).getTime();

/** A card of the api project: `planned(2, ...)` is `api#2`, and `deps` are numbers in that project. */
const planned = (n: number, s: number | null, e: number | null, deps: number[] = []) => ({
  id: cardKey("api", n),
  n,
  s,
  e,
  deps: deps.map((d) => cardKey("api", d)),
});

describe("constants", () => {
  it("lays the track out as 32 days of 40 px next to the 280 px card column", () => {
    expect(TRACK_WIDTH_PX).toBe(1280);
    expect(TOTAL_WIDTH_PX).toBe(1560);
    expect(TODAY_LINE_X_PX).toBe(280 + 14 * 40 + 20);
  });

  it("formats pixel values", () => {
    expect(px(12)).toBe("12px");
  });
});

describe("spanOf", () => {
  it("returns the card's days", () => {
    expect(spanOf(planned(1, 2, 5), null)).toEqual({ s: 2, e: 5 });
  });

  it("shifts only the card being dragged", () => {
    expect(spanOf(planned(1, 2, 5), { id: "api#1", delta: -3 })).toEqual({ s: -1, e: 2 });
    expect(spanOf(planned(1, 2, 5), { id: "api#2", delta: -3 })).toEqual({ s: 2, e: 5 });
  });

  it("counts a missing day as 0 like the design's arithmetic", () => {
    expect(spanOf(planned(1, null, null), { id: "api#1", delta: 2 })).toEqual({ s: 2, e: 2 });
  });
});

describe("bar geometry", () => {
  it("starts a bar 2 px inside its first day", () => {
    expect(barX({ s: -14, e: -14 })).toBe(2);
    expect(barX({ s: 0, e: 1 })).toBe(14 * 40 + 2);
  });

  it("is 4 px narrower than its days and never under 24 px", () => {
    expect(barWidth({ s: 0, e: 0 })).toBe(36);
    expect(barWidth({ s: 0, e: 4 })).toBe(196);
    expect(barWidth({ s: 3, e: 1 })).toBe(24);
  });

  it("snaps a pointer move to whole days", () => {
    expect(snapDays(0)).toBe(0);
    expect(snapDays(19)).toBe(0);
    expect(snapDays(21)).toBe(1);
    expect(snapDays(-120)).toBe(-3);
  });

  it("gives the lines' drawing 45 px per row", () => {
    expect(svgHeight(0)).toBe(0);
    expect(svgHeight(4)).toBe(180);
  });
});

describe("depLine", () => {
  it("goes from the end of the blocking bar to the start of the waiting one", () => {
    const line = depLine({ s: 0, e: 1 }, 0, { s: 4, e: 5 }, 2);
    expect(line).toEqual({ d: "M638 22 H648 V112 H722", bad: false });
  });

  it("marks a waiting card that starts before the blocking one ends", () => {
    expect(depLine({ s: 0, e: 4 }, 1, { s: 4, e: 6 }, 0).bad).toBe(true);
    expect(depLine({ s: 0, e: 4 }, 1, { s: 5, e: 6 }, 0).bad).toBe(false);
  });

  it("keeps the elbow 10 px right of the line's start when the target is close", () => {
    const line = depLine({ s: 2, e: 3 }, 0, { s: 0, e: 1 }, 1);
    expect(line.d).toBe("M718 22 H728 V67 H562");
    expect(line.bad).toBe(true);
  });
});

describe("dependencyLines", () => {
  const rows = [planned(1, 0, 1), planned(2, 3, 4, [1]), planned(3, 0, 2, [2, 99])];
  const span = (c: (typeof rows)[number]) => spanOf(c, null);

  it("draws one line per dependency that is on the grid, in row order", () => {
    const lines = dependencyLines(rows, span);
    expect(lines).toHaveLength(2);
    expect(lines.map((l) => l.bad)).toEqual([false, true]);
  });

  it("draws nothing when no card has a dependency", () => {
    expect(dependencyLines([planned(1, 0, 1)], span)).toEqual([]);
  });

  it("follows a dragged bar", () => {
    const dragged = (c: (typeof rows)[number]) => spanOf(c, { id: "api#2", delta: -3 });
    expect(dependencyLines(rows, dragged)[0]?.bad).toBe(true);
  });
});

describe("dayCells", () => {
  const cells = dayCells(TODAY, DAY_MS);

  it("has 32 cells, 40 px apart, starting 14 days before today", () => {
    expect(cells).toHaveLength(DAY_COUNT);
    expect(cells[1]?.x).toBe(40);
    expect(cells[0]?.date).toBe(new Date(TODAY - 14 * DAY_MS).getDate());
  });

  it("marks exactly today", () => {
    const today = cells.filter((c) => c.today);
    expect(today).toHaveLength(1);
    expect(today[0]?.date).toBe(24);
  });

  it("marks weekends", () => {
    const weekends = cells.filter((c) => c.weekend).length;
    expect(weekends).toBeGreaterThanOrEqual(8);
    expect(weekends).toBeLessThanOrEqual(10);
  });

  it("gives every cell a full date for its tooltip", () => {
    expect(cells[14]?.full).toBe(
      new Date(TODAY).toLocaleDateString(undefined, { dateStyle: "full" }),
    );
  });
});

describe("rangeLabel", () => {
  it("names the first and last day on the grid", () => {
    const f = (t: number) =>
      new Date(t).toLocaleDateString(undefined, { month: "short", day: "numeric" });
    expect(rangeLabel(TODAY, DAY_MS)).toBe(
      `${f(TODAY - 14 * DAY_MS)} to ${f(TODAY + 17 * DAY_MS)}`,
    );
  });
});
