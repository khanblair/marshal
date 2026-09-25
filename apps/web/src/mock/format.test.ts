import { describe, expect, it } from "vitest";
import { colOf, isColumn, tone } from "./constants";
import { full, money, relTime } from "./format";

const NOW = new Date("2026-09-24T10:00:00").getTime();
const S = 1000;
const MIN = 60 * S;
const H = 60 * MIN;
const D = 24 * H;

describe("money", () => {
  it.each([
    [0, "$0.00"],
    [0.004, "$0.00"],
    [0.005, "$0.01"],
    [1.1, "$1.10"],
    [3.12, "$3.12"],
    [12.345, "$12.35"],
  ])("formats %d as %s", (v, s) => {
    expect(money(v)).toBe(s);
  });
});

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

describe("full", () => {
  it("uses the locale's medium date and short time", () => {
    const expected = new Date(NOW).toLocaleString(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    });
    expect(full(NOW)).toBe(expected);
    expect(full(NOW)).toMatch(/2026/);
  });
});

describe("status helpers", () => {
  it("maps merging into the ready column", () => {
    expect(colOf("merging")).toBe("ready");
    expect(colOf("needs")).toBe("needs");
    expect(isColumn("ready")).toBe(true);
    expect(isColumn("merging")).toBe(false);
  });

  it("builds tone tokens, neutral without a tone", () => {
    expect(tone("needs-you", "subtle")).toBe("var(--color-status-needs-you-subtle)");
    expect(tone(null, "solid")).toBe("var(--color-border-strong)");
    expect(tone(null, "text")).toBe("var(--color-text-secondary)");
    expect(tone(null, "subtle")).toBe("var(--color-surface-sunken)");
  });
});
