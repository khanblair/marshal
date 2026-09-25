import { describe, expect, it } from "vitest";
import {
  type Box,
  HIGHLIGHT_PAD_PX,
  highlightBox,
  placePopover,
  popoverWidth,
  rectChanged,
  scaleRect,
  scrollDelta,
  type TourRect,
} from "./tour-geometry";

const box = (left: number, top: number, width: number, height: number): Box => ({
  left,
  top,
  width,
  height,
  bottom: top + height,
});

describe("scaleRect", () => {
  it("measures from the app root's corner", () => {
    expect(scaleRect(box(120, 80, 200, 40), box(20, 30, 1400, 800), 1)).toEqual({
      x: 100,
      y: 50,
      w: 200,
      h: 40,
    });
  });

  it("divides by the device frame's scale", () => {
    expect(scaleRect(box(60, 40, 100, 50), box(10, 20, 700, 400), 0.5)).toEqual({
      x: 100,
      y: 40,
      w: 200,
      h: 100,
    });
  });

  it("cuts a target that is taller than the root, keeping 24 px free", () => {
    expect(scaleRect(box(0, 0, 300, 2000), box(0, 0, 1000, 800), 1).h).toBe(776);
    expect(scaleRect(box(0, 0, 300, 2000), box(0, 0, 500, 400), 0.5).h).toBe(776);
  });
});

describe("rectChanged", () => {
  const before: TourRect = { x: 10, y: 20, w: 100, h: 50 };

  it("is true when there was no earlier rect", () => {
    expect(rectChanged(null, before)).toBe(true);
  });

  it("ignores a total change of 1 px or less", () => {
    expect(rectChanged(before, { ...before })).toBe(false);
    expect(rectChanged(before, { ...before, x: 10.5, w: 100.5 })).toBe(false);
  });

  it("adds up the changes on all four values", () => {
    expect(rectChanged(before, { ...before, x: 11.5 })).toBe(true);
    expect(rectChanged(before, { x: 10.5, y: 20.5, w: 100.5, h: 50 })).toBe(true);
    expect(rectChanged(before, { x: 9, y: 20, w: 100, h: 50 })).toBe(false);
  });
});

describe("highlightBox", () => {
  it("grows the target by 4 px on every side", () => {
    expect(HIGHLIGHT_PAD_PX).toBe(4);
    expect(highlightBox({ x: 10, y: 20, w: 100, h: 50 })).toEqual({ x: 6, y: 16, w: 108, h: 58 });
  });
});

describe("scrollDelta", () => {
  const area = box(0, 100, 400, 300);

  it("is null when the target is inside the scroll area", () => {
    expect(scrollDelta(box(10, 120, 50, 50), area, 1)).toBeNull();
    expect(scrollDelta(box(10, 100, 50, 300), area, 1)).toBeNull();
  });

  it("puts a target that is below the area 16 px under its top", () => {
    expect(scrollDelta(box(10, 500, 50, 50), area, 1)).toBe(384);
  });

  it("scrolls back up for a target above the area", () => {
    expect(scrollDelta(box(10, 40, 50, 50), area, 1)).toBe(-76);
  });

  it("converts screen pixels to app pixels", () => {
    expect(scrollDelta(box(10, 300, 50, 50), box(0, 100, 400, 100), 0.5)).toBe(384);
  });
});

describe("popoverWidth", () => {
  it("is 320 px, or the screen less 12 px on each side", () => {
    expect(popoverWidth(1440)).toBe(320);
    expect(popoverWidth(344)).toBe(320);
    expect(popoverWidth(300)).toBe(276);
  });
});

describe("placePopover", () => {
  const desktop = { vw: 1440, vh: 900 };

  it("sits near the middle when there is no target", () => {
    expect(placePopover({ rect: null, ...desktop })).toEqual({ x: 560, y: 360, width: 320 });
  });

  it("falls back to an 800 px screen when the height is unknown", () => {
    expect(placePopover({ rect: null, vw: 1440, vh: 0 }).y).toBe(310);
  });

  it("goes under a wide target, level with its left edge", () => {
    const rect = { x: 240, y: 100, w: 900, h: 200 };
    expect(placePopover({ rect, ...desktop })).toEqual({ x: 240, y: 312, width: 320 });
  });

  it("goes beside a narrow target when there is room on its right", () => {
    const rect = { x: 16, y: 300, w: 200, h: 40 };
    expect(placePopover({ rect, ...desktop })).toEqual({ x: 228, y: 300, width: 320 });
  });

  it("keeps a side popover at least 12 px from the top and 210 px from the bottom", () => {
    expect(placePopover({ rect: { x: 16, y: 4, w: 200, h: 40 }, ...desktop }).y).toBe(12);
    expect(placePopover({ rect: { x: 16, y: 880, w: 200, h: 40 }, ...desktop }).y).toBe(690);
  });

  it("goes under a narrow target when the right side is too tight", () => {
    const rect = { x: 1100, y: 100, w: 200, h: 40 };
    expect(placePopover({ rect, ...desktop })).toEqual({ x: 1100, y: 152, width: 320 });
  });

  it("goes above the target when there is no room below", () => {
    const rect = { x: 240, y: 600, w: 900, h: 250 };
    expect(placePopover({ rect, ...desktop })).toEqual({ x: 240, y: 398, width: 320 });
  });

  it("covers the target's top-left corner when neither side has room", () => {
    const rect = { x: 240, y: 100, w: 900, h: 700 };
    expect(placePopover({ rect, ...desktop })).toEqual({ x: 256, y: 116, width: 320 });
  });

  it("stays 12 px inside the screen on both sides", () => {
    const wide = { x: 1300, y: 100, w: 300, h: 100 };
    expect(placePopover({ rect: wide, ...desktop }).x).toBe(1108);
    const left = { x: -50, y: 100, w: 300, h: 100 };
    expect(placePopover({ rect: left, ...desktop }).x).toBe(12);
  });

  it("shrinks the popover on a narrow screen", () => {
    const placed = placePopover({ rect: { x: 20, y: 100, w: 300, h: 100 }, vw: 300, vh: 700 });
    expect(placed.width).toBe(276);
    expect(placed.x).toBe(12);
  });
});
