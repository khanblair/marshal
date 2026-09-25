import { createMemo, createSignal } from "solid-js";

/** Rows a Home list shows until "Show all" is pressed. */
export const COLLAPSED_ROW_COUNT = 5;

/** The rows to draw, and the toggle that switches between the first few and all of them. */
export function createShowAll<T>(list: () => readonly T[]) {
  const [expanded, setExpanded] = createSignal(false);
  const visible = createMemo(() => (expanded() ? list() : list().slice(0, COLLAPSED_ROW_COUNT)));
  return { expanded, visible, toggle: () => setExpanded((value) => !value) };
}
