import {
  listHeight,
  rowOffsets,
  rowsBetween,
  rowsToDraw,
  rowWindow,
  sameRange,
  scrollTopOfRow,
} from "./virtual-window";

// Ten rows of 40 px with 10 px between them: row i starts at 50 * i.
const even = rowOffsets(Array(10).fill(40), 10);

describe("rowOffsets and listHeight", () => {
  it("puts each row after the one before it and its gap, with one more entry at the end", () => {
    expect(rowOffsets([40, 100, 40], 10)).toEqual([0, 50, 160, 210]);
    expect(even).toHaveLength(11);
    expect(even[10]).toBe(500);
  });

  it("measures the list without a gap after its last row", () => {
    expect(listHeight(rowOffsets([40, 100, 40], 10), 10)).toBe(200);
    expect(listHeight(even, 10)).toBe(490);
    expect(listHeight(rowOffsets([7], 10), 10)).toBe(7);
  });

  it("has no height and no rows for an empty list", () => {
    expect(rowOffsets([], 10)).toEqual([0]);
    expect(listHeight(rowOffsets([], 10), 10)).toBe(0);
  });
});

describe("rowsBetween", () => {
  it("finds the first and last row that touch the band", () => {
    // 120 to 260 touches row 2 (100 to 140) through row 5 (250 to 290).
    expect(rowsBetween(even, 120, 260)).toEqual({ first: 2, last: 5 });
  });

  it("counts a row the band starts inside, and one the band ends inside", () => {
    expect(rowsBetween(even, 90, 101)).toEqual({ first: 1, last: 2 });
  });

  it("does not count a row that starts at the bottom edge", () => {
    expect(rowsBetween(even, 0, 100)).toEqual({ first: 0, last: 1 });
  });

  it("starts at the first row for a band above the list, and ends at the last for one below it", () => {
    expect(rowsBetween(even, -300, 20)).toEqual({ first: 0, last: 0 });
    expect(rowsBetween(even, 900, 1200)).toEqual({ first: 9, last: 9 });
  });

  it("follows rows of different heights", () => {
    const offsets = rowOffsets([40, 500, 40, 40], 10);
    // The tall row starts at 50 and ends at 550, so a band inside it touches only that row.
    expect(rowsBetween(offsets, 200, 300)).toEqual({ first: 1, last: 1 });
    expect(rowsBetween(offsets, 500, 700)).toEqual({ first: 1, last: 3 });
  });

  it("has one row for a band with no height, and no rows for an empty list", () => {
    expect(rowsBetween(even, 120, 120)).toEqual({ first: 2, last: 2 });
    expect(rowsBetween(rowOffsets([], 10), 0, 100)).toEqual({ first: 0, last: -1 });
  });
});

describe("rowWindow", () => {
  const view = { offsets: even, scrollTop: 200, viewportHeight: 100, overscan: 0 };

  it("is the rows on screen when there is no overscan", () => {
    expect(rowWindow(view)).toEqual({ first: 4, last: 5 });
  });

  it("adds the overscan above and below, in px, and stops at the ends of the list", () => {
    expect(rowWindow({ ...view, overscan: 100 })).toEqual({ first: 2, last: 7 });
    expect(rowWindow({ ...view, scrollTop: 0, overscan: 100 })).toEqual({ first: 0, last: 3 });
    expect(rowWindow({ ...view, scrollTop: 450, overscan: 400 })).toEqual({ first: 1, last: 9 });
  });

  it("draws far fewer rows than the list has", () => {
    const offsets = rowOffsets(Array(5000).fill(38), 10);
    const range = rowWindow({ offsets, scrollTop: 100_000, viewportHeight: 600, overscan: 400 });
    expect(range.last - range.first + 1).toBeLessThan(40);
    expect(range.first).toBeLessThanOrEqual(Math.floor(100_000 / 48));
    expect(range.last).toBeGreaterThanOrEqual(Math.floor(100_600 / 48));
  });

  it("changes nothing when a scroll stays inside the same rows", () => {
    const a = rowWindow({ ...view, scrollTop: 210 });
    const b = rowWindow({ ...view, scrollTop: 220 });
    expect(sameRange(a, b)).toBe(true);
    expect(sameRange(a, rowWindow({ ...view, scrollTop: 260 }))).toBe(false);
  });
});

describe("rowsToDraw", () => {
  const range = { first: 4, last: 6 };

  it("lists the range in order", () => {
    expect(rowsToDraw(range, null, 10)).toEqual([4, 5, 6]);
    expect(rowsToDraw({ first: 0, last: -1 }, null, 0)).toEqual([]);
  });

  it("keeps the row that has focus, whichever side of the range it is on", () => {
    expect(rowsToDraw(range, 1, 10)).toEqual([1, 4, 5, 6]);
    expect(rowsToDraw(range, 9, 10)).toEqual([4, 5, 6, 9]);
    expect(rowsToDraw(range, 5, 10)).toEqual([4, 5, 6]);
  });

  it("ignores a focused row the list no longer has", () => {
    expect(rowsToDraw(range, 12, 10)).toEqual([4, 5, 6]);
    expect(rowsToDraw(range, -1, 10)).toEqual([4, 5, 6]);
  });
});

describe("scrollTopOfRow", () => {
  it("is where the row starts, so scrolling there puts it at the top", () => {
    expect(scrollTopOfRow(even, 0)).toBe(0);
    expect(scrollTopOfRow(even, 7)).toBe(350);
    expect(scrollTopOfRow(rowOffsets([40, 500, 40], 10), 2)).toBe(560);
  });

  it("stays inside the list", () => {
    expect(scrollTopOfRow(even, -3)).toBe(0);
    expect(scrollTopOfRow(even, 99)).toBe(450);
    expect(scrollTopOfRow(rowOffsets([], 10), 3)).toBe(0);
  });
});
