/**
 * The one thing this app's own code is allowed to know about the desktop shell: whether it is
 * running inside one, and how to ask it to open a native folder picker. Everything else the shell
 * does (starting the daemon, signing the window in, deep links) happens entirely on the Tauri
 * side, with no coupling back into this app - this file is the single, deliberate exception,
 * because only the shell can put a native folder-picker dialog in front of a person, and a plain
 * browser cannot (`apps/desktop/src-tauri/tauri.conf.json`'s `dangerousRemoteDomainIpcAccess`
 * grants the daemon's own served page IPC access to the dialog plugin alone, nothing else).
 */

/** True while this page is running inside the desktop shell, not a plain browser tab. */
export function isDesktop(): boolean {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

/**
 * Opens the native "choose a folder" dialog and answers the folder a person picked, or null when
 * they cancelled. Outside the desktop shell, or if the shell has no dialog plugin for some
 * reason, it answers null without throwing: the caller's plain typed-path field is always there
 * as the fallback.
 */
export async function pickFolder(): Promise<string | null> {
  if (!isDesktop()) return null;
  try {
    const { open } = await import("@tauri-apps/plugin-dialog");
    const picked = await open({ directory: true, multiple: false });
    return typeof picked === "string" ? picked : null;
  } catch {
    return null;
  }
}
