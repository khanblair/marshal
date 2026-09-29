import type { Platform } from "./types";
import { createWebPlatform } from "./web";

/**
 * The desktop shell: a browser tab's abilities plus the native folder chooser. The shell puts the
 * dialog plugin in front of the person, which a plain page cannot; if the plugin is not there the
 * answer is null and the typed-path field is the way in.
 */
export function createDesktopPlatform(): Platform {
  return {
    ...createWebPlatform(),
    kind: "desktop",
    native: true,
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
