import { createSwipeHandlers, stepColumn, swipeStep } from "./swipe";

function touchEvent(x: number, y: number, kind: "start" | "end"): TouchEvent {
  const point = { clientX: x, clientY: y } as Touch;
  return (kind === "start"
    ? { touches: [point], changedTouches: [point] }
    : { touches: [], changedTouches: [point] }) as unknown as TouchEvent;
}

describe("swipeStep", () => {
  it("steps forward on a long left swipe and back on a right swipe", () => {
    expect(swipeStep(-100, 10)).toBe(1);
    expect(swipeStep(100, -10)).toBe(-1);
  });

  it("ignores short or mostly vertical moves", () => {
    expect(swipeStep(70, 0)).toBe(0);
    expect(swipeStep(-60, 0)).toBe(0);
    expect(swipeStep(100, 80)).toBe(0);
  });
});

describe("stepColumn", () => {
  const columns = ["a", "b", "c"];

  it("moves and clamps at both ends", () => {
    expect(stepColumn(columns, "b", 1)).toBe("c");
    expect(stepColumn(columns, "c", 1)).toBe("c");
    expect(stepColumn(columns, "a", -1)).toBe("a");
  });
});

describe("createSwipeHandlers", () => {
  it("reports a swipe from touch start to touch end", () => {
    const steps: number[] = [];
    const handlers = createSwipeHandlers(
      () => true,
      (step) => steps.push(step),
    );
    handlers.onTouchStart(touchEvent(300, 100, "start"));
    handlers.onTouchEnd(touchEvent(150, 110, "end"));
    expect(steps).toEqual([1]);
  });

  it("forgets the start point after a touch end", () => {
    const steps: number[] = [];
    const handlers = createSwipeHandlers(
      () => true,
      (step) => steps.push(step),
    );
    handlers.onTouchStart(touchEvent(300, 100, "start"));
    handlers.onTouchEnd(touchEvent(300, 100, "end"));
    handlers.onTouchEnd(touchEvent(100, 100, "end"));
    expect(steps).toEqual([]);
  });

  it("does nothing while disabled", () => {
    const steps: number[] = [];
    const handlers = createSwipeHandlers(
      () => false,
      (step) => steps.push(step),
    );
    handlers.onTouchStart(touchEvent(300, 100, "start"));
    handlers.onTouchEnd(touchEvent(100, 100, "end"));
    expect(steps).toEqual([]);
  });
});
