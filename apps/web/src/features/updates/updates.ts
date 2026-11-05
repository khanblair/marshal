import { platform } from "~/platform";
import { createUpdateStore, type UpdateInvoke } from "./update-store";

type NativeWindow = Window & { __TAURI_INTERNALS__?: { invoke: UpdateInvoke } };
const native = typeof window !== "undefined" && platform().kind === "desktop";
const bridge = native ? (window as NativeWindow).__TAURI_INTERNALS__ : undefined;
export const updates = createUpdateStore(bridge ? (command) => bridge.invoke(command) : null);
const releasesUrl = "https://github.com/khanblair/marshal/releases";
export function openReleases() {
  void platform().openExternal(releasesUrl);
}
