import type { Platform } from "./types";
import { createWebPlatform, openInNewTab } from "./web";

/** The shell's IPC bridge, which the shell puts on the page it shows. */
type TauriWindow = Window & {
  __TAURI_INTERNALS__?: { invoke?: (command: string, args?: unknown) => Promise<unknown> };
};

/**
 * Asks the shell to open an address in the system browser. The desktop window has no new windows,
 * so a tab the page opens itself goes nowhere there. A shell that refuses answers false.
 */
async function openWithShell(url: string): Promise<boolean> {
  try {
    const internals = (window as TauriWindow).__TAURI_INTERNALS__;
    if (!internals?.invoke) return false;
    await internals.invoke("plugin:shell|open", { path: url });
    return true;
  } catch {
    return false;
  }
}

/**
 * The desktop shell: a browser tab's abilities plus the native folder chooser. The shell puts the
 * dialog plugin in front of the person, which a plain page cannot; if the plugin is not there the
 * answer is null and the typed-path field is the way in. Addresses open in the system browser.
 */
export function createDesktopPlatform(): Platform {
  return {
    ...createWebPlatform(),
    kind: "desktop",
    native: true,
    async openExternal(url) {
      return (await openWithShell(url)) || openInNewTab(url);
    },
    async pickFolder() {
      try {
        const { open } = await import("@tauri-apps/plugin-dialog");
        const picked = await open({ directory: true, multiple: false });
        return typeof picked === "string" ? picked : null;
      } catch {
        return null;
      }
    },
  };
}
