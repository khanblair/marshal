const SWIPE_MIN_DISTANCE_PX = 70;
/** A swipe must be clearly more horizontal than vertical. */
const SWIPE_HORIZONTAL_RATIO = 1.5;

/** -1 (previous column), 0 (not a swipe), or 1 (next column) for a finger's travel. */
export function swipeStep(dx: number, dy: number): -1 | 0 | 1 {
  const across = Math.abs(dx);
  if (across > SWIPE_MIN_DISTANCE_PX && across > Math.abs(dy) * SWIPE_HORIZONTAL_RATIO) {
    return dx < 0 ? 1 : -1;
  }
  return 0;
}

/** The column `step` places away from `current`, clamped to the first and last. */
export function stepColumn<T>(columns: readonly T[], current: T, step: number): T {
  const target = Math.max(0, Math.min(columns.length - 1, columns.indexOf(current) + step));
  return columns[target] ?? current;
}

export interface SwipeHandlers {
  onTouchStart: (e: TouchEvent) => void;
  onTouchEnd: (e: TouchEvent) => void;
}

/**
 * Touch handlers for one board. Each board keeps its own start point, because the board can be
 * mounted twice (main pane and split pane).
 */
export function createSwipeHandlers(
  enabled: () => boolean,
  onStep: (step: -1 | 1) => void,
): SwipeHandlers {
  let start: { x: number; y: number } | null = null;
  return {
    onTouchStart: (e) => {
      const touch = e.touches[0];
      if (touch) start = { x: touch.clientX, y: touch.clientY };
    },
    onTouchEnd: (e) => {
      const touch = e.changedTouches[0];
      if (!enabled() || !start || !touch) return;
      const step = swipeStep(touch.clientX - start.x, touch.clientY - start.y);
      if (step !== 0) onStep(step);
      start = null;
    },
  };
}
