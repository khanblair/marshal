import type { PickOptions, Platform } from "./types";

const VIBRATE_MS: Record<"tap" | "success" | "error", number> = { tap: 10, success: 20, error: 40 };

/** Opens the browser's file chooser from an input that is never put on the page. */
export function pickFilesWithInput(options: PickOptions = {}): Promise<File[]> {
  return new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    if (options.accept) input.accept = options.accept;
    if (options.multiple) input.multiple = true;
    if (options.camera) input.setAttribute("capture", "environment");
    input.addEventListener("change", () => resolve([...(input.files ?? [])]), { once: true });
    input.addEventListener("cancel", () => resolve([]), { once: true });
    input.click();
  });
}

/** A plain browser tab: a file chooser and, where the device has it, a short vibration. */
export function createWebPlatform(): Platform {
  return {
    kind: "web",
    native: false,
    pickFiles: pickFilesWithInput,
    pickFolder: () => Promise.resolve(null),
    canScanCode: false,
    scanCode: () => Promise.resolve(null),
    haptic(kind) {
      if (typeof navigator !== "undefined" && "vibrate" in navigator) {
        navigator.vibrate(VIBRATE_MS[kind]);
      }
    },
    onDeepLink: () => () => undefined,
  };
}
