import { describe, expect, it } from "vitest";
import { relTime } from "./format";

const NOW = new Date("2026-09-24T10:00:00").getTime();
const S = 1000;
const MIN = 60 * S;
const H = 60 * MIN;
const D = 24 * H;

describe("relTime", () => {
  it.each([
    [0, "Just now"],
    [44 * S, "Just now"],
    [45 * S, "1 min ago"],
    [90 * S, "2 min ago"],
    [59 * MIN, "59 min ago"],
    [H - 1, "60 min ago"],
    [H, "1 h ago"],
    [23 * H, "23 h ago"],
    [D - 1, "24 h ago"],
    [D, "Yesterday"],
    [1.4 * D, "Yesterday"],
    [1.5 * D, "2 days ago"],
    [3 * D, "3 days ago"],
  ])("%d ms ago reads %s", (ago, text) => {
    expect(relTime(NOW - ago, NOW)).toBe(text);
  });
});
