/** A box as the browser reports it. `DOMRect` fits. */
export interface Box {
  left: number;
  top: number;
  width: number;
  height: number;
  bottom: number;
}

/** A target's position and size in the app's own pixels, relative to the app root. */
export interface TourRect {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** Room left below the target so the cut-out never reaches the bottom edge, in px. */
const HEIGHT_MARGIN_PX = 24;
/** A target that moved less than this in total is not re-measured, in px. */
const MOVE_THRESHOLD_PX = 1;
/** How far the cut-out grows past the target on every side, in px. */
export const HIGHLIGHT_PAD_PX = 4;
/** Space kept between a scrolled target and the top of its scroll area, in px. */
const SCROLL_MARGIN_PX = 16;

/**
 * Turns a target's box into app pixels. `scale` is the device frame's scale: the browser
 * reports scaled pixels, the app lays out in its own. A tall target is cut short so the
 * cut-out stays inside the root.
 */
export function scaleRect(target: Box, root: Box, scale: number): TourRect {
  return {
    x: (target.left - root.left) / scale,
    y: (target.top - root.top) / scale,
    w: target.width / scale,
    h: Math.min(target.height / scale, root.height / scale - HEIGHT_MARGIN_PX),
  };
}

/** True when there was no earlier rect, or the target moved or resized by more than 1 px in total. */
export function rectChanged(previous: TourRect | null, next: TourRect): boolean {
  if (!previous) return true;
  const change =
    Math.abs(previous.x - next.x) +
    Math.abs(previous.y - next.y) +
    Math.abs(previous.w - next.w) +
    Math.abs(previous.h - next.h);
  return change > MOVE_THRESHOLD_PX;
}

/** The cut-out: the target grown by 4 px on every side. */
export function highlightBox(rect: TourRect): { x: number; y: number; w: number; h: number } {
  return {
    x: rect.x - HIGHLIGHT_PAD_PX,
    y: rect.y - HIGHLIGHT_PAD_PX,
    w: rect.w + 2 * HIGHLIGHT_PAD_PX,
    h: rect.h + 2 * HIGHLIGHT_PAD_PX,
  };
}

/**
 * How far to scroll a scroll area so the target is visible, or null when it already is.
 * The target's top ends up 16 px below the top of the area.
 */
export function scrollDelta(target: Box, area: Box, scale: number): number | null {
  if (target.top >= area.top && target.bottom <= area.bottom) return null;
  return (target.top - area.top) / scale - SCROLL_MARGIN_PX;
}

const POPOVER_MAX_WIDTH_PX = 320;
const VIEWPORT_MARGIN_PX = 12;
const POPOVER_HEIGHT_PX = 190;
/** Gap between the target and the popover, in px. */
const POPOVER_GAP_PX = 12;
/** A target narrower than this gets the popover beside it, not under it. */
const NARROW_TARGET_PX = 280;
/** Tallest a side-placed popover can start: the popover's height plus its margin, in px. */
const SIDE_BOTTOM_ROOM_PX = 210;
/** Inset of the popover from the target's corner when nothing else fits, in px. */
const OVERLAP_INSET_PX = 16;
const FALLBACK_VIEWPORT_HEIGHT_PX = 800;
/** Where the centered popover sits above the middle of the screen, in px. */
const CENTER_LIFT_PX = 90;

export interface Placement {
  x: number;
  y: number;
  width: number;
}

/** A popover is 320 px wide, or the screen width less a 12 px margin on each side. */
export const popoverWidth = (vw: number): number =>
  Math.min(POPOVER_MAX_WIDTH_PX, vw - 2 * VIEWPORT_MARGIN_PX);

function centered(vw: number, vh: number, width: number): Placement {
  return { x: (vw - width) / 2, y: vh / 2 - CENTER_LIFT_PX, width };
}

function besideOrBelow(rect: TourRect, vw: number, vh: number, width: number): Placement {
  const below = rect.y + rect.h + POPOVER_GAP_PX;
  const above = rect.y - POPOVER_GAP_PX - POPOVER_HEIGHT_PX;
  const right = rect.x + rect.w + POPOVER_GAP_PX;
  let x: number;
  let y: number;
  if (rect.w < NARROW_TARGET_PX && right + width < vw - VIEWPORT_MARGIN_PX) {
    x = right;
    y = Math.max(VIEWPORT_MARGIN_PX, Math.min(vh - SIDE_BOTTOM_ROOM_PX, rect.y));
  } else if (below + POPOVER_HEIGHT_PX < vh) {
    x = rect.x;
    y = below;
  } else if (above > VIEWPORT_MARGIN_PX) {
    x = rect.x;
    y = above;
  } else {
    x = rect.x + OVERLAP_INSET_PX;
    y = rect.y + OVERLAP_INSET_PX;
  }
  return {
    x: Math.max(VIEWPORT_MARGIN_PX, Math.min(vw - width - VIEWPORT_MARGIN_PX, x)),
    y,
    width,
  };
}

/**
 * Where the popover goes on tablet and desktop. Beside a narrow target if there is room to
 * its right; otherwise under the target, above it, or over its top-left corner. Without a
 * target it sits near the middle of the screen. The x position stays 12 px inside the screen.
 */
export function placePopover(input: { rect: TourRect | null; vw: number; vh: number }): Placement {
  const height = input.vh || FALLBACK_VIEWPORT_HEIGHT_PX;
  const width = popoverWidth(input.vw);
  if (!input.rect) return centered(input.vw, height, width);
  return besideOrBelow(input.rect, input.vw, height, width);
}
