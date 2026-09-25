import type { State } from "../state-types";
import type { ResolvedTheme } from "../types";

const DARK_QUERY = "(prefers-color-scheme: dark)";

const systemIsDark = (): boolean =>
  typeof window.matchMedia === "function" && window.matchMedia(DARK_QUERY).matches;

/** Sets `data-theme` on the document from `S.theme`; "system" follows the OS setting. */
export function applyTheme(S: State): void {
  let theme: ResolvedTheme;
  if (S.theme === "system") theme = systemIsDark() ? "dark" : "light";
  else theme = S.theme;
  document.documentElement.setAttribute("data-theme", theme);
  S.resolvedTheme = theme;
}

/** Re-applies the theme when the OS switches between light and dark. */
export function watchSystemTheme(S: State): void {
  if (typeof window.matchMedia !== "function") return;
  window.matchMedia(DARK_QUERY).addEventListener("change", () => applyTheme(S));
}
