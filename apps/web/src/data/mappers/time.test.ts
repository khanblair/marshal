import { describe, expect, it } from "vitest";
import { relTime } from "~/data/format";
import { toAgo, toMillis } from "./time";

const STAMP = "2026-09-25T10:15:30.123Z";
const AT = Date.UTC(2026, 8, 25, 10, 15, 30, 123);
const MINUTE = 60_000;

describe("toMillis", () => {
  it("reads a UTC timestamp with milliseconds", () => {
    expect(toMillis(STAMP)).toBe(AT);
  });

  it("keeps the milliseconds", () => {
    expect(toMillis("2026-09-25T10:15:30.001Z") - toMillis("2026-09-25T10:15:30.000Z")).toBe(1);
  });

  it("refuses a value that is not a time, so a daemon bug is not shown as NaN", () => {
    expect(() => toMillis("yesterday")).toThrow(RangeError);
    expect(() => toMillis("")).toThrow('"" is not a timestamp');
  });
});

describe("toAgo", () => {
  it.each([
    [10_000, "Just now"],
    [4 * MINUTE, "4 min ago"],
    [61 * MINUTE, "1 h ago"],
    [26 * 60 * MINUTE, "Yesterday"],
  ])("says %d ms after the time as %s", (later, words) => {
    expect(toAgo(STAMP, { now: () => AT + later })).toBe(words);
  });

  it("uses the words of the mock's relative time, so the screens read the same", () => {
    const now = AT + 7 * MINUTE;
    expect(toAgo(STAMP, { now: () => now })).toBe(relTime(AT, now));
  });

  it("follows the clock it is given, which is the daemon's", () => {
    let now = AT;
    const clock = { now: () => now };
    expect(toAgo(STAMP, clock)).toBe("Just now");
    now += 3 * MINUTE;
    expect(toAgo(STAMP, clock)).toBe("3 min ago");
  });
});
