import { createDesktopPlatform } from "./desktop";
import { createMobilePlatform } from "./mobile";
import type { Platform, PlatformKind } from "./types";
import { createWebPlatform } from "./web";

export type { Haptic, PickOptions, Platform, PlatformKind } from "./types";

const PHONE_AGENT = /Android|iPhone|iPad|iPod/i;

/** Which kind of device this is. Only a native shell that reports a phone is the phone app. */
export function detectKind(env: { tauri: boolean; userAgent: string }): PlatformKind {
  if (!env.tauri) return "web";
  return PHONE_AGENT.test(env.userAgent) ? "mobile" : "desktop";
}

const factories: Record<PlatformKind, () => Platform> = {
  web: createWebPlatform,
  desktop: createDesktopPlatform,
  mobile: createMobilePlatform,
};

/** Builds the adapter for a kind of device. */
export const createPlatform = (kind: PlatformKind): Platform => factories[kind]();

let current: Platform | null = null;

/** The adapter for the device the app is running on, chosen once on first use. */
export function platform(): Platform {
  current ??= createPlatform(
    detectKind({
      tauri: typeof window !== "undefined" && "__TAURI_INTERNALS__" in window,
      userAgent: typeof navigator === "undefined" ? "" : navigator.userAgent,
    }),
  );
  return current;
}
