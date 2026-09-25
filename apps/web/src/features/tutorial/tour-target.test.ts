import { afterEach, describe, expect, it } from "vitest";
import { findTarget, measureTarget, scrollTargetIntoView } from "./tour-target";

type Box = [left: number, top: number, width: number, height: number];

function place(el: Element, [left, top, width, height]: Box): void {
  el.getBoundingClientRect = () =>
    ({
      left,
      top,
      width,
      height,
      right: left + width,
      bottom: top + height,
      x: left,
      y: top,
    }) as DOMRect;
}

function add(parent: Element, tour: string | null, tag = "div"): HTMLElement {
  const el = document.createElement(tag);
  if (tour) el.setAttribute("data-tour", tour);
  parent.append(el);
  return el;
}

afterEach(() => {
  document.body.replaceChildren();
});

describe("findTarget", () => {
  it("finds an element by its data-tour value", () => {
    const el = add(document.body, "tiles");
    place(el, [0, 0, 100, 50]);
    expect(findTarget(["tiles"])).toBe(el);
  });

  it("skips a name that is missing or has no size and tries the next", () => {
    const hidden = add(document.body, "projects");
    place(hidden, [0, 0, 0, 0]);
    const phone = add(document.body, "projects-phone");
    place(phone, [0, 0, 40, 40]);
    expect(findTarget(["projects", "projects-phone"])).toBe(phone);
    expect(findTarget(["nope", "projects-phone"])).toBe(phone);
  });

  it("returns null when nothing matches", () => {
    const flat = add(document.body, "tiles");
    place(flat, [0, 0, 100, 0]);
    expect(findTarget(["tiles"])).toBeNull();
    expect(findTarget(["absent"])).toBeNull();
  });
});

describe("measureTarget", () => {
  function app(): HTMLElement {
    const root = add(document.body, null);
    root.setAttribute("data-app-root", "1");
    place(root, [10, 20, 1000, 700]);
    return root;
  }

  it("gives the target's box relative to the app root", () => {
    const target = add(app(), "needs");
    place(target, [110, 220, 300, 100]);
    expect(measureTarget(["needs"], 1, false)).toEqual({ x: 100, y: 200, w: 300, h: 100 });
  });

  it("is null without a target or without an app root", () => {
    expect(measureTarget(["needs"], 1, false)).toBeNull();
    const stray = add(document.body, "needs");
    place(stray, [0, 0, 10, 10]);
    expect(measureTarget(["needs"], 1, false)).toBeNull();
  });

  it("applies the device frame scale", () => {
    const target = add(app(), "needs");
    place(target, [60, 120, 150, 50]);
    expect(measureTarget(["needs"], 0.5, false)).toEqual({ x: 100, y: 200, w: 300, h: 100 });
  });
});

describe("scrollTargetIntoView", () => {
  function scrollArea(root: Element, height: number, top: number, overflowY: string) {
    const area = add(root, null);
    area.style.overflowY = overflowY;
    Object.defineProperty(area, "scrollHeight", { value: 1000 });
    Object.defineProperty(area, "clientHeight", { value: height });
    place(area, [0, top, 400, height]);
    return area;
  }

  it("scrolls the nearest scrollable ancestor so the target sits 16 px below its top", () => {
    const root = add(document.body, null);
    const area = scrollArea(root, 300, 100, "auto");
    const target = add(area, "needs");
    place(target, [0, 500, 100, 50]);
    scrollTargetIntoView(target, root, 1);
    expect(area.scrollTop).toBe(384);
  });

  it("leaves an area alone when the target is already visible", () => {
    const root = add(document.body, null);
    const area = scrollArea(root, 300, 100, "scroll");
    const target = add(area, "needs");
    place(target, [0, 150, 100, 50]);
    scrollTargetIntoView(target, root, 1);
    expect(area.scrollTop).toBe(0);
  });

  it("skips ancestors that do not scroll and stops at the app root", () => {
    const root = add(document.body, null);
    const outer = scrollArea(document.body, 300, 100, "auto");
    outer.append(root);
    const quiet = add(root, null);
    const target = add(quiet, "needs");
    place(target, [0, 900, 100, 50]);
    scrollTargetIntoView(target, root, 1);
    expect(outer.scrollTop).toBe(0);
  });

  it("only tries the nearest scrollable ancestor", () => {
    const root = add(document.body, null);
    const outer = scrollArea(root, 300, 100, "auto");
    const inner = scrollArea(outer, 300, 100, "auto");
    const target = add(inner, "needs");
    place(target, [0, 200, 100, 50]);
    scrollTargetIntoView(target, root, 1);
    expect(inner.scrollTop).toBe(0);
    expect(outer.scrollTop).toBe(0);
  });
});
