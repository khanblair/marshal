import type { AuthorizeURL } from "@marshal/protocol";
import { platform } from "~/platform";

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
