import { describe, expect, it } from "vitest";
import { contrastRatio, measureAll, pairKey } from "../src/contrast.ts";
import { contrastExceptions } from "../src/contrast-exceptions.ts";

describe("contrastRatio", () => {
  it("is 21 for black on white and 1 for identical colors", () => {
    expect(contrastRatio("#000000", "#FFFFFF")).toBeCloseTo(21, 5);
    expect(contrastRatio("#16191D", "#16191D")).toBeCloseTo(1, 5);
  });

  it("is symmetric", () => {
    expect(contrastRatio("#6A727B", "#EEF0F2")).toBeCloseTo(contrastRatio("#EEF0F2", "#6A727B"), 10);
  });
});

describe("design contrast rules", () => {
  const failing = measureAll().filter((p) => p.ratio + 1e-9 < p.min);

  it("measures every pair in both themes", () => {
    const themes = new Set(measureAll().map((p) => p.theme));
    expect(themes).toEqual(new Set(["light", "dark"]));
  });

  it("lists every failing pair as an accepted exception, and nothing else", () => {
    expect(new Set(failing.map(pairKey))).toEqual(new Set(contrastExceptions));
  });

  it("keeps primary text and ink readable on every surface", () => {
    const strict = measureAll().filter((p) => p.fg === "color-text-primary" || p.fg === "color-ink");
    expect(strict.every((p) => p.ratio >= p.min)).toBe(true);
  });
});
