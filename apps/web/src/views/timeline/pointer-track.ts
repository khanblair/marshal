export interface PointerTrackHandlers {
  /** Horizontal distance from the press, on every move. */
  move: (dx: number) => void;
  /** The pointer was released. */
  up: () => void;
  /** The browser took the pointer away (for example a system gesture). */
  cancel: () => void;
}

/**
 * Follows a pressed pointer on `window`, as the design's bar drag does, so the drag keeps
 * going when the pointer leaves the bar. Listeners are removed on release, on cancel,
 * or when the returned stop function is called, whichever comes first.
 */
export function trackPointer(startX: number, on: PointerTrackHandlers): () => void {
  const move = (ev: PointerEvent): void => on.move(ev.clientX - startX);
  const stop = (): void => {
    window.removeEventListener("pointermove", move);
    window.removeEventListener("pointerup", up);
    window.removeEventListener("pointercancel", cancel);
  };
  function up(): void {
    stop();
    on.up();
  }
  function cancel(): void {
    stop();
    on.cancel();
  }
  window.addEventListener("pointermove", move);
  window.addEventListener("pointerup", up);
  window.addEventListener("pointercancel", cancel);
  return stop;
}
