import { type JSX, onCleanup } from "solid-js";
import { M } from "~/mock";
import { clampDetailWidth, DETAIL_MAX_PX, DETAIL_MIN_PX, detailWidth } from "./shell-layout";

/** How far an arrow key moves the panel edge. */
const KEY_STEP_PX = 24;

/**
 * The 5 px bar between the main view and the card panel. Drag it, or press the left
 * and right arrow keys, to change the panel's width.
 */
export function ResizeHandle() {
  let stopDrag: (() => void) | undefined;
  onCleanup(() => stopDrag?.());

  const onPointerDown: JSX.EventHandler<HTMLDivElement, PointerEvent> = (event) => {
    event.preventDefault();
    stopDrag?.();
    const startX = event.clientX;
    const startWidth = detailWidth();
    // The frame may be scaled to fit the window, so screen pixels differ from layout pixels.
    const scale = M._scale || 1;
    const move = (e: PointerEvent) => {
      M.S.detailW = clampDetailWidth(startWidth - (e.clientX - startX) / scale);
    };
    const end = () => stopDrag?.();
    stopDrag = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", end);
      stopDrag = undefined;
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", end);
  };

  const onKeyDown: JSX.EventHandler<HTMLDivElement, KeyboardEvent> = (event) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const step = event.key === "ArrowLeft" ? KEY_STEP_PX : -KEY_STEP_PX;
    M.S.detailW = clampDetailWidth(detailWidth() + step);
  };

  return (
    // biome-ignore lint/a11y/useSemanticElements: a splitter is a focusable separator that handles keys and drags; an <hr> cannot take focus
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize card panel"
      aria-valuemin={DETAIL_MIN_PX}
      aria-valuemax={DETAIL_MAX_PX}
      aria-valuenow={detailWidth()}
      tabindex={0}
      onPointerDown={onPointerDown}
      onKeyDown={onKeyDown}
      class="w-[5px] flex-none cursor-col-resize bg-border relative z-raised hover:bg-border-strong"
    />
  );
}
