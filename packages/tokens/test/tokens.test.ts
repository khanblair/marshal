import { describe, expect, it } from "vitest";
import { themes } from "../src/themes.ts";
import { breakpoints, layers, radii, textSizes, textSizesByBand } from "../src/tokens.ts";

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
  it("uses the six standard size bands", () => {
    expect(breakpoints).toEqual({ sm: 640, md: 768, lg: 1024, xl: 1280, "2xl": 1536 });
  });

  it("keeps breakpoints ascending", () => {
    const values = Object.values(breakpoints);
    expect([...values].sort((a, b) => a - b)).toEqual(values);
  });

  it("only shrinks known text sizes in the bands that name them", () => {
    for (const [band, sizes] of Object.entries(textSizesByBand)) {
      expect(["base", ...Object.keys(breakpoints)]).toContain(band);
      for (const [name, px] of Object.entries(sizes)) {
        expect(px).toBeLessThan(textSizes[name as keyof typeof textSizes]);
      }
    }
  });

  it("orders text sizes, radii, and layers sensibly", () => {
    expect(textSizes.micro).toBeLessThan(textSizes.tile);
    expect(radii.xs).toBeLessThan(radii.xl);
    expect(layers.dialog).toBeLessThan(layers.palette);
    expect(layers.toast).toBeLessThan(layers.onboarding);
  });
});
