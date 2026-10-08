import { createSignal } from "solid-js";

/** Where this device remembers that the card's details are folded away. */
const STORAGE_KEY = "marshal.card.details.collapsed";

function remembered(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "1";
  } catch {
    return false;
  }
}

const [collapsed, setCollapsed] = createSignal(remembered());

/**
 * Whether the open card's worktree, members, actions, and settings are folded away, so the tab
 * under them has the height. It is one choice for every card, kept on this device.
 */
export const detailsCollapsed = collapsed;

/** Folds the card's details away, or brings them back. */
export function toggleDetails(): void {
  const next = !collapsed();
  setCollapsed(next);
  try {
    if (next) window.localStorage.setItem(STORAGE_KEY, "1");
    else window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Without storage the choice lasts until the page is closed.
  }
}
