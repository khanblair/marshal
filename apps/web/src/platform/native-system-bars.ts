type TauriWindow = Window & {
  __TAURI_INTERNALS__?: {
    invoke?: (command: string, args?: unknown) => Promise<unknown>;
  };
};

/** Updates native system glyphs to contrast with the resolved app theme, when Android supports it. */
export function setNativeSystemBars(dark: boolean): void {
  if (typeof window === "undefined" || typeof navigator === "undefined") return;
  if (!/Android/i.test(navigator.userAgent)) return;

  const internals = (window as TauriWindow).__TAURI_INTERNALS__;
  if (!internals?.invoke) return;

  try {
    // Older APKs do not have this command. The app theme remains usable without it.
    void internals.invoke("plugin:marshal-mobile|set_system_bars", { dark }).catch(() => {});
  } catch {
    // A missing or unsupported native bridge must never interrupt theme application.
  }
}
