import { describe, expect, it } from "vitest";
import { themes } from "../src/themes.ts";
import { breakpoints, layers, radii, textSizes } from "../src/tokens.ts";

describe("themes", () => {
  it("defines the same color tokens in light and dark", () => {
    expect(Object.keys(themes.dark.colors)).toEqual(Object.keys(themes.light.colors));
    expect(Object.keys(themes.dark.charts)).toEqual(Object.keys(themes.light.charts));
  });

  it("uses six-digit hex for every color token", () => {
    for (const theme of Object.values(themes)) {
      for (const value of Object.values(theme.colors)) expect(value).toMatch(/^#[0-9A-F]{6}$/);
    }
  });
});

describe("scales", () => {
  it("keeps breakpoints ascending", () => {
    const values = Object.values(breakpoints);
    expect([...values].sort((a, b) => a - b)).toEqual(values);
  });

  it("orders text sizes, radii, and layers sensibly", () => {
    expect(textSizes.micro).toBeLessThan(textSizes.tile);
    expect(radii.xs).toBeLessThan(radii.xl);
    expect(layers.dialog).toBeLessThan(layers.palette);
    expect(layers.toast).toBeLessThan(layers.onboarding);
  });
});
