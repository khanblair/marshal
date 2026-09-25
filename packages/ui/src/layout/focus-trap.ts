/** What the design's `trapKey` treats as focusable inside a dialog. */
const FOCUSABLE = 'button:not([disabled]),input,select,textarea,[tabindex="0"]';

/** Wait before moving focus, so the dialog is painted first (as the design does). */
export const FOCUS_DELAY_MS = 20;

/**
 * Keeps Tab and Shift+Tab inside `box`, wrapping from the last control to the
 * first and back. Returns true when it moved focus.
 */
export function trapTab(event: KeyboardEvent, box: HTMLElement): boolean {
  if (event.key !== "Tab") return false;
  const items = Array.from(box.querySelectorAll<HTMLElement>(FOCUSABLE));
  const first = items[0];
  const last = items[items.length - 1];
  if (!first || !last) return false;
  const active = document.activeElement;
  if (event.shiftKey && active === first) {
    event.preventDefault();
    last.focus();
    return true;
  }
  if (!event.shiftKey && active === last) {
    event.preventDefault();
    first.focus();
    return true;
  }
  return false;
}

/** Focuses the first `[data-autofocus]` element in `box`, if any. */
export function focusInitial(box: HTMLElement): void {
  box.querySelector<HTMLElement>("[data-autofocus]")?.focus();
}
