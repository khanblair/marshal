import { createRoot } from "solid-js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { useChartWidth } from "./use-chart-width";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

type Observer = { callback: () => void; observed: Element[]; disconnected: boolean };
let observers: Observer[] = [];

class FakeResizeObserver {
  state: Observer;
  constructor(callback: () => void) {
    this.state = { callback, observed: [], disconnected: false };
    observers.push(this.state);
  }
  observe(element: Element) {
    this.state.observed.push(element);
  }
  disconnect() {
    this.state.disconnected = true;
  }
}

/** A grid whose first child is `widthPx` wide, as `getBoundingClientRect` reports it. */
function gridOf(widthPx: () => number): HTMLElement {
  const grid = document.createElement("div");
  const first = document.createElement("div");
  first.getBoundingClientRect = () => ({ width: widthPx() }) as DOMRect;
  grid.append(first);
  return grid;
}

function mount(grid: HTMLElement) {
  return createRoot((dispose) => {
    const chart = useChartWidth();
    chart.ref(grid);
    return { chart, dispose };
  });
}

beforeEach(() => {
  observers = [];
  vi.useFakeTimers();
  vi.stubGlobal("ResizeObserver", FakeResizeObserver);
  M._scale = 1;
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  M._scale = 1;
});

describe("useChartWidth", () => {
  it("starts at 520 px", () => {
    const { chart, dispose } = mount(gridOf(() => 300));
    expect(chart.width()).toBe(520);
    dispose();
  });

  it("measures the first column 30 ms after mounting and watches the grid", () => {
    let width = 358.7;
    const grid = gridOf(() => width);
    const { chart, dispose } = mount(grid);
    vi.advanceTimersByTime(29);
    expect(chart.width()).toBe(520);
    vi.advanceTimersByTime(1);
    expect(chart.width()).toBe(358);
    expect(observers[0]?.observed).toEqual([grid]);
    width = 700;
    observers[0]?.callback();
    expect(chart.width()).toBe(700);
    dispose();
  });

  it("ignores changes of two pixels or less, and empty boxes", () => {
    let width = 522;
    const { chart, dispose } = mount(gridOf(() => width));
    vi.advanceTimersByTime(30);
    expect(chart.width()).toBe(520);
    width = 0;
    observers[0]?.callback();
    expect(chart.width()).toBe(520);
    width = 523;
    observers[0]?.callback();
    expect(chart.width()).toBe(523);
    dispose();
  });

  it("divides by the device frame's scale", () => {
    M._scale = 0.5;
    const { chart, dispose } = mount(gridOf(() => 300));
    vi.advanceTimersByTime(30);
    expect(chart.width()).toBe(600);
    dispose();
  });

  it("stops watching when its owner is disposed, before or after the first measure", () => {
    const first = mount(gridOf(() => 300));
    first.dispose();
    vi.advanceTimersByTime(60);
    expect(observers[0]?.observed).toEqual([]);
    expect(first.chart.width()).toBe(520);
    const second = mount(gridOf(() => 300));
    vi.advanceTimersByTime(30);
    second.dispose();
    expect(observers[1]?.disconnected).toBe(true);
  });

  it("works without ResizeObserver", () => {
    vi.stubGlobal("ResizeObserver", undefined);
    const { chart, dispose } = mount(gridOf(() => 400));
    vi.advanceTimersByTime(30);
    expect(chart.width()).toBe(400);
    dispose();
  });
});
