import { createSignal } from "solid-js";

/** The former shared preference remains a fallback for desktop installs. */
const LEGACY_STORAGE_KEY = "marshal.card.details.collapsed";
const STORAGE_KEYS = {
  desktop: "marshal.card.details.collapsed.desktop",
  mobile: "marshal.card.details.collapsed.mobile",
} as const;

function remembered(mobile: boolean): boolean {
  try {
    const saved = window.localStorage.getItem(mobile ? STORAGE_KEYS.mobile : STORAGE_KEYS.desktop);
    if (saved === "1") return true;
    if (saved === "0") return false;
    if (mobile) return true;
    return window.localStorage.getItem(LEGACY_STORAGE_KEY) === "1";
  } catch {
    return mobile;
  }
}

const [desktopCollapsed, setDesktopCollapsed] = createSignal(remembered(false));
const [mobileCollapsed, setMobileCollapsed] = createSignal(remembered(true));

/**
 * Whether the open card's metadata, members, actions, and settings are folded away. Mobile and
 * desktop keep independent choices on this device; mobile starts folded for more chat space.
 */
export function detailsCollapsed(mobile = false): boolean {
  return mobile ? mobileCollapsed() : desktopCollapsed();
}

/** Folds the card's details away, or brings them back. */
export function toggleDetails(mobile = false): void {
  const next = !detailsCollapsed(mobile);
  (mobile ? setMobileCollapsed : setDesktopCollapsed)(next);
  try {
    window.localStorage.setItem(
      mobile ? STORAGE_KEYS.mobile : STORAGE_KEYS.desktop,
      next ? "1" : "0",
    );
  } catch {
    // Without storage the choice lasts until the page is closed.
  }
}
