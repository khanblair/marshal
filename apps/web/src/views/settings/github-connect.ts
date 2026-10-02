import { type Accessor, createSignal, onCleanup, onMount } from "solid-js";
import type { GitHubConnection } from "~/data/mappers/integrations";
import { M } from "~/mock";
import { platform } from "~/platform";
import { cancelGitHubConnect, readGitHubConnect, startGitHubConnect } from "./integration-actions";

/** How often a sign-in that is waiting on GitHub is asked how it is going. */
export const POLL_MS = 2000;
/** How long the dialog keeps looking for a new installation after "Add an account". */
export const WATCH_MS = 60_000;

/**
 * A tab opened inside the click, before anything is awaited, so the browser does not take it for a
 * pop-up. The desktop and phone shells have no new windows, so they get none and use the system
 * browser later (`platform().openExternal`).
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

export interface GitHubConnectController {
  /** What the daemon last said, or null until the first answer. */
  connection: Accessor<GitHubConnection | null>;
  /** The daemon's sentence for the last call it refused, or "". */
  error: Accessor<string>;
  /** True from the click on Sign in until the daemon has answered it. */
  starting: Accessor<boolean>;
  /** Starts a sign-in. Call it from the click itself: the tab has to open before any wait. */
  signIn(): void;
  /** Opens an address in the person's own browser, from a click. */
  open(url: string): void;
  /** Opens the install page and looks for the new installation for a minute. */
  addAccount(url: string): void;
  /** Reads the connection once, and keeps reading while a sign-in is waiting on GitHub. */
  refresh(): Promise<void>;
  /** Puts the code on the clipboard and says so. */
  copyCode(code: string): void;
}

/**
 * The sign-in dialog's whole conversation with the daemon: it reads the connection when the dialog
 * opens, starts a sign-in, follows it every couple of seconds while GitHub is the one being waited
 * on, and cancels a pending one when the dialog goes away.
 *
 * One timer chain, not an interval, so a slow read cannot overlap the next; a newer call's answer
 * wins over an older one that was still on its way.
 */
export function createGitHubConnect(): GitHubConnectController {
  const [connection, setConnection] = createSignal<GitHubConnection | null>(null);
  const [error, setError] = createSignal("");
  const [starting, setStarting] = createSignal(false);
  let tab: Window | null = null;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let watchTimer: ReturnType<typeof setTimeout> | undefined;
  let watching = false;
  let alive = true;
  let latest = 0;

  const waiting = (): boolean => {
    const state = connection()?.state;
    return state === "pending" || state === "needs_install" || watching;
  };

  /** Sends the sign-in tab on, or the system browser where the app has no tabs. */
  const goTo = (url: string): void => {
    if (tab && !tab.closed) {
      try {
        tab.location.href = url;
        return;
      } catch {
        // A tab that cannot be sent on is left to the buttons, which are always on screen.
      }
    }
    if (platform().native) void platform().openExternal(url);
  };

  const schedule = (): void => {
    clearTimeout(timer);
    if (alive && waiting()) timer = setTimeout(() => void refresh(), POLL_MS);
  };

  /** Takes a daemon answer, and sends the tab on to the install page when sign-in has just finished. */
  const accept = (next: GitHubConnection): void => {
    const wasPending = connection()?.state === "pending";
    setConnection(next);
    if (wasPending && next.state === "needs_install" && next.installUrl) goTo(next.installUrl);
  };

  async function refresh(): Promise<void> {
    const mine = ++latest;
    const answer = await readGitHubConnect();
    if (!alive || mine !== latest) return;
    // A read that failed keeps what is shown and asks again, so one bad answer is not a dead end.
    if (!("error" in answer)) accept(answer);
    schedule();
  }

  const copyCode = (code: string): void => {
    if (!navigator.clipboard) {
      M.toast("Copy the code by hand.");
      return;
    }
    navigator.clipboard.writeText(code).then(
      () => M.toast("Code copied"),
      () => M.toast("Copy the code by hand."),
    );
  };

  const signIn = (): void => {
    tab = openBlankTab();
    setError("");
    setStarting(true);
    const mine = ++latest;
    void startGitHubConnect().then((answer) => {
      setStarting(false);
      if (!alive) {
        // The dialog went away while the daemon was starting the sign-in, so nobody is left to end it.
        if (!("error" in answer) && answer.state === "pending") void cancelGitHubConnect();
        tab?.close();
        return;
      }
      if (mine !== latest) return;
      if ("error" in answer) {
        setError(answer.error);
        tab?.close();
        return;
      }
      accept(answer);
      schedule();
      if (answer.state === "pending" && answer.userCode) {
        // Best effort: the code is on screen too, with a button to copy it.
        navigator.clipboard?.writeText(answer.userCode).catch(() => undefined);
        if (answer.verificationUri) goTo(answer.verificationUri);
      }
    });
  };

  const open = (url: string): void => {
    void platform().openExternal(url);
  };

  const addAccount = (url: string): void => {
    open(url);
    watching = true;
    clearTimeout(watchTimer);
    watchTimer = setTimeout(() => {
      watching = false;
    }, WATCH_MS);
    schedule();
  };

  onMount(() => void refresh());
  onCleanup(() => {
    alive = false;
    clearTimeout(timer);
    clearTimeout(watchTimer);
    if (connection()?.state === "pending") void cancelGitHubConnect();
  });

  return { connection, error, starting, signIn, open, addAccount, refresh, copyCode };
}
