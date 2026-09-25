import { describe, expect, it } from "vitest";
import { colOf, isColumn, tone } from "./constants";
import { full, money } from "./format";

const NOW = new Date("2026-09-24T10:00:00").getTime();

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
