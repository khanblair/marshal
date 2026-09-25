import { scaleRect, scrollDelta, type TourRect } from "./tour-geometry";

/** The first `data-tour` element among `names` that is on screen with a size, or null. */
export function findTarget(names: readonly string[]): Element | null {
  for (const name of names) {
    const el = document.querySelector(`[data-tour="${name}"]`);
    if (!el) continue;
    const box = el.getBoundingClientRect();
    if (box.width > 0 && box.height > 0) return el;
  }
  return null;
}

const SCROLLABLE = /auto|scroll/;

/**
 * Scrolls the nearest scrollable ancestor (below the app root) so the target shows. Only the
 * nearest one is tried, even when it already shows the target.
 */
export function scrollTargetIntoView(target: Element, root: Element, scale: number): void {
  for (let area = target.parentElement; area && area !== root; area = area.parentElement) {
    if (
      !SCROLLABLE.test(getComputedStyle(area).overflowY) ||
      area.scrollHeight <= area.clientHeight
    ) {
      continue;
    }
    const delta = scrollDelta(target.getBoundingClientRect(), area.getBoundingClientRect(), scale);
    if (delta !== null) area.scrollTop += delta;
    return;
  }
}

/**
 * Finds and measures the target, in the app's own pixels. Null when no target is on screen or
 * the app root is missing. `scroll` brings the target into view first.
 */
export function measureTarget(
  names: readonly string[],
  scale: number,
  scroll: boolean,
): TourRect | null {
  const target = findTarget(names);
  const root = document.querySelector("[data-app-root]");
  if (!target || !root) return null;
  if (scroll) scrollTargetIntoView(target, root, scale);
  return scaleRect(target.getBoundingClientRect(), root.getBoundingClientRect(), scale);
}
