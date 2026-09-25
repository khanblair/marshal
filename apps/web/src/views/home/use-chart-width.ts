import { createSignal, onCleanup, onMount } from "solid-js";
import { M } from "~/mock";

/** Width used until the first measure, as in the design. */
const INITIAL_WIDTH_PX = 520;
/** The design waits a moment after mounting before it starts measuring. */
const FIRST_MEASURE_DELAY_MS = 30;
/** Changes of one or two pixels are ignored, so a scroll bar appearing does not loop. */
const MIN_WIDTH_CHANGE_PX = 2;

/**
 * The width of the chart columns, for sizing the SVGs. Give `ref` to the grid
 * that holds the charts; the width follows its first child, in the app's own
 * pixels (the device frame scales the app).
 */
export function useChartWidth() {
  const [width, setWidth] = createSignal(INITIAL_WIDTH_PX);
  let grid: HTMLElement | undefined;

  const measure = () => {
    if (!grid) return;
    const column = grid.firstElementChild ?? grid;
    const measured = Math.floor(column.getBoundingClientRect().width / (M._scale || 1));
    if (measured > 0 && Math.abs(measured - width()) > MIN_WIDTH_CHANGE_PX) setWidth(measured);
  };

  onMount(() => {
    const observer =
      typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(measure);
    const timer = setTimeout(() => {
      if (grid) observer?.observe(grid);
      measure();
    }, FIRST_MEASURE_DELAY_MS);
    onCleanup(() => {
      clearTimeout(timer);
      observer?.disconnect();
    });
  });

  return {
    width,
    ref: (element: HTMLElement) => {
      grid = element;
    },
  };
}
