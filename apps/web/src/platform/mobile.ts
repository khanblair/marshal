import type { Haptic, Platform } from "./types";
import { createWebPlatform } from "./web";

/** Runs a native call and drops any failure: a missing plugin must never break a screen. */
async function quietly<T>(run: () => Promise<T>): Promise<T | null> {
  try {
    return await run();
  } catch {
    return null;
  }
}

async function tap(kind: Haptic): Promise<void> {
  const haptics = await import("@tauri-apps/plugin-haptics");
  if (kind === "tap") await haptics.impactFeedback("light");
  else await haptics.notificationFeedback(kind);
}

async function scan(): Promise<string | null> {
  const scanner = await import("@tauri-apps/plugin-barcode-scanner");
  if ((await scanner.checkPermissions()) !== "granted") {
    if ((await scanner.requestPermissions()) !== "granted") return null;
  }
  const scanned = await scanner.scan({ formats: [scanner.Format.QRCode] });
  return scanned.content || null;
}

/**
 * The phone app: a browser's file chooser (the phone's own, which offers the camera too), plus the
 * camera reader, haptics, and `marshal://` links from the Tauri plugins. Every plugin is loaded on
 * first use, so a build without one still opens.
 */
export function createMobilePlatform(): Platform {
  return {
    ...createWebPlatform(),
    kind: "mobile",
    native: true,
    canScanCode: true,
    scanCode: () => quietly(scan),
    haptic(kind) {
      void quietly(() => tap(kind));
    },
    onDeepLink(handler) {
      let stop: (() => void) | null = null;
      let stopped = false;
      void quietly(async () => {
        const links = await import("@tauri-apps/plugin-deep-link");
        for (const url of (await links.getCurrent()) ?? []) handler(url);
        const off = await links.onOpenUrl((urls) => {
          for (const url of urls) handler(url);
        });
        if (stopped) off();
        else stop = off;
      });
      return () => {
        stopped = true;
        stop?.();
      };
    },
  };
}
