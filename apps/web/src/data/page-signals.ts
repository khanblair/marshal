/** The parts of `window` and `document` that the page signals use, so a test can hand in fakes. */
export interface PageTargets {
  window: Pick<EventTarget, "addEventListener" | "removeEventListener">;
  document: Pick<EventTarget, "addEventListener" | "removeEventListener"> & {
    readonly visibilityState: string;
  };
}

/**
 * Calls a function when the page becomes visible again, or when the browser says the network is
 * back. A tab that was hidden for a while has a stale picture of the daemon, and a phone that
 * lost its signal has a dropped connection, so both are good moments to look again.
 */
export function watchPage(targets: PageTargets): (onReturn: () => void) => () => void {
  return (onReturn) => {
    const onVisibility = () => {
      if (targets.document.visibilityState === "visible") onReturn();
    };
    targets.document.addEventListener("visibilitychange", onVisibility);
    targets.window.addEventListener("online", onReturn);
    return () => {
      targets.document.removeEventListener("visibilitychange", onVisibility);
      targets.window.removeEventListener("online", onReturn);
    };
  };
}
