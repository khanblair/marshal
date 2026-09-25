import { applyTheme, watchSystemTheme } from "./dom/theme";
import { watchResize } from "./dom/viewport";
import type { Marshal } from "./marshal";

declare global {
  interface Window {
    M?: Marshal;
  }
}

/**
 * What the browser does once the store exists: puts it on `window.M`, applies the theme, follows
 * the window's size and the system theme, and announces it. The store is ready when it has the
 * daemon's first snapshots, so nothing here marks it ready.
 */
export function attachToPage(M: Marshal): Marshal {
  window.M = M;
  applyTheme(M.S);
  watchSystemTheme(M.S);
  watchResize(M);
  window.dispatchEvent(new Event("marshal-ready"));
  return M;
}
