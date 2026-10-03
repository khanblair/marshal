import type { AuthorizeURL } from "@marshal/protocol";
import { onCleanup } from "solid-js";
import { platform } from "~/platform";

/** How often a row is asked whether Google has been granted, while the person is on Google's page. */
const WAIT_MS = 2000;
/** How long it keeps asking. A person who has not finished by then reads the row again by hand. */
const WAIT_LIMIT_MS = 120_000;

/**
 * Asks the connection list again every couple of seconds after Google's page was opened, because
 * nothing tells this page when the grant lands in the other tab. It stops when the row reads
 * connected, after two minutes, or when the component goes away. Call it while the component is set up.
 */
export function createGrantWatcher(
  isConnected: () => boolean,
  refresh: () => Promise<void>,
): { start: () => void; stop: () => void } {
  let timer: ReturnType<typeof setInterval> | undefined;
  const stop = (): void => {
    if (timer) clearInterval(timer);
    timer = undefined;
  };
  onCleanup(stop);
  const start = (): void => {
    stop();
    const started = Date.now();
    timer = setInterval(() => {
      void refresh();
      if (isConnected() || Date.now() - started > WAIT_LIMIT_MS) stop();
    }, WAIT_MS);
  };
  return { start, stop };
}

/**
 * A tab opened inside the click, before anything is awaited, so the browser does not take it for a
 * pop-up. The desktop and phone shells have no new windows, so they get none and use the system
 * browser afterwards (`platform().openExternal`).
 */
function openBlankTab(): Window | null {
  if (platform().native) return null;
  const tab = window.open("about:blank", "_blank");
  if (!tab) return null;
  try {
    tab.opener = null;
  } catch {
    // A tab that cannot be cut loose from this page is still a tab.
  }
  return tab;
}

/**
 * Sends the person to Google's consent page in their own browser. Call it from the click itself: the
 * tab has to open before the address is fetched. False means there was no address to go to, and
 * then the tab is closed again.
 */
export async function openGoogleConsent(
  fetchUrl: () => Promise<AuthorizeURL | null>,
): Promise<boolean> {
  const tab = openBlankTab();
  const answer = await fetchUrl();
  if (!answer) {
    tab?.close();
    return false;
  }
  if (tab) {
    tab.location.href = answer.url;
    return true;
  }
  return platform().openExternal(answer.url);
}
