// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the GitHub connection is the daemon's).
import { daemon } from "~/testing/daemon-integrations-store";
import type { GitHubConnect, TestCheck } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { unwrap } from "solid-js/store";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { createIntegrationStore } from "~/testing/fake-integrations";
import { POLL_MS, WATCH_MS } from "./github-connect";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/** The prototype's own sentence for the GitHub row, which nothing on the daemon ever answers. */
const PROTOTYPE_DETAIL = "GitHub App installed on 3 repositories";
const FIX = "Grant pull request access to the App on GitHub.";
const CODE = "WDJB-MJHT";
const VERIFY_URL = "https://github.com/login/device";
const INSTALL_URL = "https://github.com/apps/marshal-kanban/installations/new";
/** A token as a person pastes it. Synthetic: the fake daemon never reaches GitHub. */
const TOKEN = "ghp_synthetic0token0for0tests";
const CONNECT_ROUTE = "/v1/integrations/github/connect";

/** The store's own rows as the seed made it, so a reset can put back the app's words. */
const INITIAL = structuredClone(unwrap(M.S.integrations));

/**
 * The routes the app asked for while it came online, kept before any test clears the call list. The
 * boot is over by the time the first test runs, so this is the only place it can be seen.
 */
let boot: string[] = [];
beforeAll(() => {
  boot = daemon.routes();
});

/**
 * Puts the daemon and the store back to the state they start in, so one test cannot see another's
 * save. `connected` seeds the GitHub row as set up, which is what shows the Manage button.
 */
function resetIntegrations(connected = false): void {
  Object.assign(daemon.integrations, createIntegrationStore());
  daemon.calls.length = 0;
  if (connected) {
    const row = daemon.integrations.rows.find((integration) => integration.id === "github");
    if (row) {
      row.st = "connected";
      row.detail = PROTOTYPE_DETAIL;
    }
  }
  const rows = structuredClone(INITIAL).map((row) =>
    row.id === "github" && connected
      ? { ...row, st: "connected" as const, detail: PROTOTYPE_DETAIL }
      : row,
  );
  M.set({ integrations: rows, settingsSection: "integrations", toasts: [] });
}

/** A tab as `window.open` hands it back: it can be navigated, closed, and cut loose from the page. */
interface FakeTab {
  closed: boolean;
  opener: unknown;
  location: { href: string };
  close: () => void;
}
const newTab = (): FakeTab => ({
  closed: false,
  opener: {},
  location: { href: "about:blank" },
  close: vi.fn(),
});
/** Makes `window.open` answer this tab, or none (a blocked pop-up, or a shell with no tabs). */
function openAnswers(tab: FakeTab | null) {
  return vi.spyOn(window, "open").mockReturnValue(tab as unknown as Window | null);
}

const writeText = vi.fn<(text: string) => Promise<void>>();

beforeEach(() => {
  vi.useFakeTimers();
  writeText.mockReset().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
  resetIntegrations();
});
afterEach(() => {
  cleanup();
  window.localStorage.removeItem("marshal.integrations.view");
  window.localStorage.removeItem("marshal.integrations.expanded");
  vi.restoreAllMocks();
  vi.useRealTimers();
  Reflect.deleteProperty(navigator, "clipboard");
});

const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);
const githubRow = () => M.S.integrations.find((integration) => integration.id === "github");
/** The GitHub row's own panel, so a button is found in that row rather than in another one. */
const githubPanel = (): HTMLElement => {
  const panel = screen.getByText("GitHub").closest("[data-integration]");
  if (!(panel instanceof HTMLElement)) throw new Error("the GitHub row is not drawn");
  return panel;
};
const rowButton = (name: string): HTMLElement =>
  within(githubPanel()).getByRole("button", { name });
const dialog = (): HTMLElement => screen.getByRole("dialog", { name: "Connect GitHub" });
const button = (name: string): HTMLElement => within(dialog()).getByRole("button", { name });
const has = (text: string | RegExp): boolean => within(dialog()).queryByText(text) !== null;
const click = (element: HTMLElement): void => {
  fireEvent.click(element);
};
const count = (route: string): number => daemon.routes().filter((entry) => entry === route).length;
/** Lets fake time pass, and every answer that is waiting on it arrive. */
const pass = async (ms: number): Promise<void> => {
  await vi.advanceTimersByTimeAsync(ms);
};
const see = (text: string | RegExp) =>
  vi.waitFor(() => expect(within(dialog()).getByText(text)).toBeInTheDocument());

/**
 * Opens the GitHub dialog and waits for its first read of the connection to be answered.
 * `connected` starts with GitHub set up, by a sign-in or by a token.
 */
async function openDialog(connected = false, mode: "oauth" | "token" = "oauth"): Promise<void> {
  if (connected) resetIntegrations(true);
  daemon.integrations.github.mode = mode;
  render(() => <SettingsView />);
  click(rowButton(connected ? "Manage" : "Connect"));
  await vi.waitFor(() => expect(count(`GET ${CONNECT_ROUTE}`)).toBe(1));
  await pass(0);
}

/** Starts a sign-in from the idle panel and waits until the code is on screen. */
async function signIn(): Promise<void> {
  click(button("Sign in with GitHub"));
  await see(CODE);
}

const answer = (fields: Partial<GitHubConnect> & Pick<GitHubConnect, "state">): GitHubConnect => ({
  installations: [],
  ...fields,
});
const needsInstall = answer({ state: "needs_install", login: "ada", installUrl: INSTALL_URL });
const connectedAs = (...accounts: [string, boolean][]) =>
  answer({
    state: "connected",
    mode: "oauth",
    login: "ada",
    installUrl: INSTALL_URL,
    installations: accounts.map(([account, allRepositories]) => ({
      account,
      kind: "user" as const,
      allRepositories,
    })),
  });

describe("the Integrations section on the daemon", () => {
  it("shows every connection the daemon knows, with the daemon's own state for GitHub", () => {
    render(() => <SettingsView />);
    // Trello, the calendar, Gmail, Telegram, and Discord are listed and read the way the daemon's
    // own `known()` reports them and the way their own mock rows already read: their own sections
    // are still the mock's. GitHub and Obsidian (S29a, S29b) are this fixture's own two daemon-backed
    // rows - this store pins several other sections to mock (S17, S20, S7c, S9) but pins neither
    // S29a nor S29b, so both read the daemon's answer here rather than the seed's.
    for (const name of ["GitHub", "Trello", "Google Calendar", "Gmail", "Telegram", "Discord"]) {
      expect(screen.getByText(name)).toBeInTheDocument();
    }
    expect(githubRow()?.st).toBe("none");
    // Nothing is stored for GitHub, so the prototype's own "installed" sentence does not show.
    expect(screen.queryByText(PROTOTYPE_DETAIL)).not.toBeInTheDocument();
    expect(rowButton("Connect")).toBeInTheDocument();
    // A later phase's row keeps the prototype's own state until its own phase builds it; Obsidian,
    // this run's own switched row, reads connected (it is self-owned, never "none") and so also
    // shows Manage.
    expect(screen.getAllByRole("button", { name: "Manage" }).length).toBeGreaterThan(0);
    // The daemon was asked for the list while the app came online.
    expect(boot).toContain("GET /v1/integrations");
  });

  it("opens a dialog instead of a form, on the sign-in tab, and asks where the connection is", async () => {
    await openDialog();
    expect(dialog()).toBeInTheDocument();
    expect(within(dialog()).getByRole("tab", { name: "Sign in with GitHub" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(within(dialog()).getByRole("tab", { name: "Paste a token" })).toHaveAttribute(
      "aria-selected",
      "false",
    );
    expect(
      has(
        "Sign in with your GitHub account. Marshal opens GitHub, shows a code, and does the rest.",
      ),
    ).toBe(true);
    expect(button("Sign in with GitHub")).toBeEnabled();
    // The old App form's fields are gone for good.
    expect(screen.queryByLabelText("App ID", { exact: false })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Private key", { exact: false })).not.toBeInTheDocument();
  });

  it("closes from its button and from Escape, and ends nothing when nothing was pending", async () => {
    await openDialog();
    click(button("Close"));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    click(rowButton("Connect"));
    fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await pass(0);
    expect(daemon.routes()).not.toContain(`DELETE ${CONNECT_ROUTE}`);
  });

  it("opens a tab inside the click, before the daemon answers, then sends it to GitHub with the code copied", async () => {
    const tab = newTab();
    const open = openAnswers(tab);
    await openDialog();
    click(button("Sign in with GitHub"));
    // The tab was opened by the click itself: nothing has been awaited yet, and no code is shown.
    expect(open).toHaveBeenCalledTimes(1);
    expect(open).toHaveBeenCalledWith("about:blank", "_blank");
    expect(has(CODE)).toBe(false);
    expect(tab.opener).toBeNull();
    await see(CODE);
    expect(daemon.routes()).toContain(`POST ${CONNECT_ROUTE}`);
    expect(tab.location.href).toBe(VERIFY_URL);
    expect(writeText).toHaveBeenCalledWith(CODE);
    // The code and both buttons stay on screen whatever the tab did.
    expect(has("Type this code on GitHub and approve. This screen updates by itself.")).toBe(true);
    expect(button("Open GitHub")).toBeEnabled();
    expect(button("Copy code")).toBeEnabled();
    expect(has(VERIFY_URL)).toBe(true);
  });

  it("copies the code from its button, and says so, or says to copy it by hand", async () => {
    openAnswers(newTab());
    await openDialog();
    await signIn();
    writeText.mockClear();
    click(button("Copy code"));
    await vi.waitFor(() => expect(toasts()).toContain("Code copied"));
    expect(writeText).toHaveBeenCalledWith(CODE);
    writeText.mockRejectedValue(new Error("denied"));
    click(button("Copy code"));
    await vi.waitFor(() => expect(toasts()).toContain("Copy the code by hand."));
  });

  it("still shows the code and the buttons when the browser gave no tab, and opens GitHub from the button", async () => {
    const open = openAnswers(null);
    await openDialog();
    await signIn();
    expect(button("Open GitHub")).toBeInTheDocument();
    expect(button("Copy code")).toBeInTheDocument();
    expect(open).toHaveBeenCalledTimes(1);
    click(button("Open GitHub"));
    expect(open).toHaveBeenLastCalledWith(VERIFY_URL, "_blank");
  });

  it("shows the daemon's sentence and closes the empty tab when a sign-in cannot start", async () => {
    const tab = newTab();
    openAnswers(tab);
    await openDialog();
    daemon.integrations.github.startRefusal = "GitHub is down.";
    click(button("Sign in with GitHub"));
    await see("GitHub is down.");
    expect(tab.close).toHaveBeenCalled();
    // The sign-in was never started, so the idle panel is still there to try again.
    expect(button("Sign in with GitHub")).toBeEnabled();
    expect(has(CODE)).toBe(false);
  });

  it("polls every two seconds, sends the open tab on to the install page, then shows the connection", async () => {
    const tab = newTab();
    openAnswers(tab);
    await openDialog();
    await signIn();
    daemon.integrations.github.script.push(
      needsInstall,
      connectedAs(["ada", true], ["acme", false]),
    );
    const reads = () => count(`GET ${CONNECT_ROUTE}`);
    const before = reads();
    // Half the interval is too soon for a read; the whole of it is not.
    await pass(POLL_MS / 2);
    expect(reads()).toBe(before);
    await pass(POLL_MS);
    expect(reads()).toBe(before + 1);
    await see(
      "Signed in as @ada. Install Marshal Kanban on your account, choosing All repositories.",
    );
    // The tab the person approved the code in is taken to the page that installs the App.
    expect(tab.location.href).toBe(INSTALL_URL);
    expect(button("Install Marshal Kanban")).toBeEnabled();
    await pass(POLL_MS);
    await see("Connected as @ada");
    expect(has("@ada, all repositories")).toBe(true);
    expect(has("@acme, selected repositories")).toBe(true);
    expect(button("Add an account or organization")).toBeEnabled();
    expect(button("Test connection")).toBeEnabled();
    expect(button("Disconnect")).toBeEnabled();
    // The daemon connected it, so the card's own row says so too.
    await vi.waitFor(() => expect(githubRow()?.st).toBe("connected"));
    expect(rowButton("Manage")).toBeInTheDocument();
    // Connected is the end of the sign-in: nothing more is asked for.
    const settled = reads();
    await pass(POLL_MS * 5);
    expect(reads()).toBe(settled);
  });

  it("shows an Install button, and leaves the tab alone, when the tab was closed before the install step", async () => {
    const tab = newTab();
    openAnswers(tab);
    await openDialog();
    await signIn();
    tab.closed = true;
    daemon.integrations.github.script.push(needsInstall);
    await pass(POLL_MS);
    await see(/Signed in as @ada\./);
    // Nothing was navigated: the closed tab still holds the code page it was sent to.
    expect(tab.location.href).toBe(VERIFY_URL);
    const open = vi.mocked(window.open);
    click(button("Install Marshal Kanban"));
    expect(open).toHaveBeenLastCalledWith(INSTALL_URL, "_blank");
    expect(has(INSTALL_URL)).toBe(true);
  });

  it("shows the Install button when there was never a tab", async () => {
    openAnswers(null);
    await openDialog();
    await signIn();
    daemon.integrations.github.script.push(needsInstall);
    await pass(POLL_MS);
    await vi.waitFor(() => expect(button("Install Marshal Kanban")).toBeEnabled());
  });

  it("warns to check the GitHub account before approving the code", async () => {
    openAnswers(newTab());
    await openDialog();
    await signIn();
    expect(
      has(/Before you approve, check that GitHub shows the account that owns your repositories\./),
    ).toBe(true);
  });

  it("says an organization owner must approve, and lets the person switch account, before the install", async () => {
    openAnswers(newTab());
    await openDialog();
    await signIn();
    daemon.integrations.github.script.push(needsInstall);
    await pass(POLL_MS);
    await see(/Signed in as @ada\./);
    expect(
      has("On an organization you do not own, an owner has to approve the install first."),
    ).toBe(true);
    expect(has(/Not @ada\?/)).toBe(true);
    click(button("Switch account"));
    await see(CODE);
    expect(count(`POST ${CONNECT_ROUTE}`)).toBe(2);
  });

  it("offers Switch account on a connection, which starts a new sign-in without disconnecting", async () => {
    openAnswers(newTab());
    await openDialog();
    await signIn();
    daemon.integrations.github.script.push(connectedAs(["ada", true]));
    await pass(POLL_MS);
    await see("Connected as @ada");
    expect(has(/Not @ada\?/)).toBe(true);
    click(button("Switch account"));
    await see(CODE);
    expect(count(`POST ${CONNECT_ROUTE}`)).toBe(2);
    expect(daemon.routes()).not.toContain("DELETE /v1/integrations/github");
  });

  it.each<[GitHubConnect["state"], string]>([
    ["denied", "You refused the code on GitHub, so nothing was connected."],
    ["expired", "The code ran out before it was approved."],
    ["failed", "The sign-in did not finish."],
  ])("says a %s sign-in did not connect, and offers Try again", async (state, sentence) => {
    openAnswers(newTab());
    await openDialog();
    await signIn();
    daemon.integrations.github.script.push(answer({ state }));
    await pass(POLL_MS);
    await see(sentence);
    expect(has(CODE)).toBe(false);
    // The polling stopped with the sign-in.
    const reads = count(`GET ${CONNECT_ROUTE}`);
    await pass(POLL_MS * 3);
    expect(count(`GET ${CONNECT_ROUTE}`)).toBe(reads);
    // Try again opens a new tab inside its own click and starts a second sign-in.
    click(button("Try again"));
    expect(vi.mocked(window.open)).toHaveBeenCalledTimes(2);
    await see(CODE);
    expect(count(`POST ${CONNECT_ROUTE}`)).toBe(2);
  });

  it("shows the daemon's own sentence for a sign-in that ended, in place of the default", async () => {
    openAnswers(newTab());
    await openDialog();
    await signIn();
    daemon.integrations.github.script.push(
      answer({ state: "failed", message: "GitHub turned the code down." }),
    );
    await pass(POLL_MS);
    await see("GitHub turned the code down.");
    expect(has("The sign-in did not finish.")).toBe(false);
  });

  it("cancels a pending sign-in when the dialog is closed, and stops asking", async () => {
    openAnswers(newTab());
    await openDialog();
    await signIn();
    click(button("Close"));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await vi.waitFor(() => expect(daemon.routes()).toContain(`DELETE ${CONNECT_ROUTE}`));
    expect(daemon.integrations.github.flow).toBeNull();
    const reads = count(`GET ${CONNECT_ROUTE}`);
    await pass(POLL_MS * 5);
    expect(count(`GET ${CONNECT_ROUTE}`)).toBe(reads);
  });

  it("cancels a pending sign-in when the dialog is closed with Escape", async () => {
    openAnswers(newTab());
    await openDialog();
    await signIn();
    fireEvent.keyDown(dialog(), { key: "Escape" });
    await vi.waitFor(() => expect(daemon.routes()).toContain(`DELETE ${CONNECT_ROUTE}`));
  });

  it("does not cancel anything once the sign-in has moved on to the install step", async () => {
    openAnswers(newTab());
    await openDialog();
    await signIn();
    daemon.integrations.github.script.push(needsInstall);
    await pass(POLL_MS);
    await see(/Signed in as @ada\./);
    click(button("Close"));
    await pass(0);
    expect(daemon.routes()).not.toContain(`DELETE ${CONNECT_ROUTE}`);
  });

  it("resumes a sign-in the daemon already has under way, and follows it", async () => {
    // A sign-in started before the page was reloaded is still pending on the daemon.
    daemon.integrations.github.flow = answer({
      state: "pending",
      userCode: CODE,
      verificationUri: VERIFY_URL,
    });
    const open = openAnswers(null);
    render(() => <SettingsView />);
    click(rowButton("Connect"));
    await see(CODE);
    // Nothing was opened for the person: the dialog only shows where it is.
    expect(open).not.toHaveBeenCalled();
    daemon.integrations.github.script.push(connectedAs(["ada", true]));
    await pass(POLL_MS);
    await see("Connected as @ada");
  });
});

describe("a connected GitHub in the dialog", () => {
  it("shows who it is connected as, each installation, and the buttons that manage it", async () => {
    await openDialog(true);
    expect(has("Connected as @ada")).toBe(true);
    expect(has("@ada, all repositories")).toBe(true);
    for (const name of ["Add an account or organization", "Test connection", "Disconnect"]) {
      expect(button(name)).toBeEnabled();
    }
    // Nothing is waiting on GitHub, so the dialog asks once and then leaves it be.
    await pass(POLL_MS * 5);
    expect(count(`GET ${CONNECT_ROUTE}`)).toBe(1);
  });

  it("runs the stored connection's test and shows the checks it found", async () => {
    await openDialog(true);
    click(button("Test connection"));
    await vi.waitFor(() => expect(toasts()).toContain("GitHub test passed"));
    expect(
      daemon.calls.some(
        (call) => call.method === "POST" && call.url.endsWith("/v1/integrations/github/test"),
      ),
    ).toBe(true);
    expect(within(dialog()).getByText(/A ping reached Marshal/)).toBeInTheDocument();
  });

  it("marks the row as needing attention when a test finds a bad permission, and shows the fix", async () => {
    await openDialog(true);
    daemon.integrations.checks = (): TestCheck[] => [
      {
        name: "Permissions",
        state: "failed",
        message: "The App cannot read pull requests.",
        fix: FIX,
      },
    ];
    click(button("Test connection"));
    await vi.waitFor(() => expect(toasts()).toContain("GitHub test failed"));
    await see(FIX);
    expect(githubRow()?.st).toBe("error");
    // An error row offers Reconnect rather than Manage, so a person is invited back to the dialog.
    expect(rowButton("Reconnect")).toBeInTheDocument();
  });

  it("forgets the connection after asking, and goes back to the sign-in", async () => {
    await openDialog(true);
    click(button("Disconnect"));
    // The confirm says what is discarded rather than promising a backup: the daemon keeps none.
    expect(M.S.dialog?.title).toBe("Disconnect GitHub");
    M.S.dialog?.run();
    await vi.waitFor(() => expect(toasts()).toContain("GitHub disconnected"));
    expect(daemon.routes()).toContain("DELETE /v1/integrations/github");
    await vi.waitFor(() => expect(githubRow()?.st).toBe("none"));
    expect(screen.queryByText(PROTOTYPE_DETAIL)).not.toBeInTheDocument();
    // The dialog read the connection again and now offers the sign-in.
    await see(
      "Sign in with your GitHub account. Marshal opens GitHub, shows a code, and does the rest.",
    );
    expect(button("Sign in with GitHub")).toBeEnabled();
    expect(rowButton("Connect")).toBeInTheDocument();
  });

  it("opens the install page from Add an account, and looks for the new installation for a minute", async () => {
    const open = openAnswers(null);
    await openDialog(true);
    click(button("Add an account or organization"));
    expect(open).toHaveBeenCalledWith(INSTALL_URL, "_blank");
    daemon.integrations.github.script.push(connectedAs(["ada", true], ["acme", true]));
    await pass(POLL_MS);
    await see("@acme, all repositories");
    // It keeps asking while the minute lasts, and not after it.
    const asked = () => count(`GET ${CONNECT_ROUTE}`);
    const during = asked();
    await pass(POLL_MS * 5);
    expect(asked()).toBeGreaterThan(during);
    await pass(WATCH_MS);
    const after = asked();
    await pass(POLL_MS * 5);
    expect(asked()).toBe(after);
  });

  it("stops asking when the dialog is closed during the minute", async () => {
    openAnswers(null);
    await openDialog(true);
    click(button("Add an account or organization"));
    await pass(POLL_MS);
    click(button("Close"));
    const asked = count(`GET ${CONNECT_ROUTE}`);
    await pass(POLL_MS * 5);
    expect(count(`GET ${CONNECT_ROUTE}`)).toBe(asked);
  });

  it("says that a token on the other tab replaces the sign-in", async () => {
    await openDialog(true);
    expect(has("Saving a token on the other tab replaces this sign-in.")).toBe(true);
  });
});

describe("pasting a token in the dialog", () => {
  const tokenField = (): HTMLElement =>
    within(dialog()).getByLabelText(/Personal access token|Replace token/);
  const paste = (value = TOKEN): void => {
    fireEvent.input(tokenField(), { target: { value } });
  };
  const openTokenTab = async (connected = false, mode: "oauth" | "token" = "oauth") => {
    await openDialog(connected, mode);
    click(within(dialog()).getByRole("tab", { name: "Paste a token" }));
  };

  it("shows the token field with its help text, hidden as a password", async () => {
    await openTokenTab();
    expect(tokenField()).toHaveAttribute("type", "password");
    expect(
      has(
        "Use a classic token with the repo scope, or a fine-grained token with read and write access to Contents, Pull requests, Issues and Actions, and read access to Checks. Marshal keeps it in the OS keychain.",
      ),
    ).toBe(true);
    expect(button("Test")).toBeEnabled();
    expect(button("Save")).toBeEnabled();
  });

  it("tests a token and shows the checks without saving anything", async () => {
    await openTokenTab();
    paste();
    click(button("Test"));
    await see(/A ping reached Marshal/);
    expect(daemon.bodies("POST /v1/integrations/github/token/test")).toEqual([{ token: TOKEN }]);
    expect(has("The GitHub App works and Marshal can use it.")).toBe(true);
    // Nothing was saved: no save went up, and the row is still not connected.
    expect(daemon.routes()).not.toContain("PUT /v1/integrations/github/token");
    expect(githubRow()?.st).toBe("none");
  });

  it("says what is wrong with a token the test refuses", async () => {
    await openTokenTab();
    daemon.integrations.refuseSave = () => "GitHub did not accept that token.";
    paste();
    click(button("Test"));
    await see("Make a new token on GitHub and paste it here.");
    expect(has("GitHub did not accept that token.")).toBe(true);
    expect(githubRow()?.st).toBe("none");
  });

  it("asks for a token before it asks the daemon anything", async () => {
    await openTokenTab();
    click(button("Save"));
    expect(has("Paste a GitHub token first.")).toBe(true);
    click(button("Test"));
    expect(has("Paste a GitHub token first.")).toBe(true);
    await pass(0);
    expect(daemon.routes()).not.toContain("PUT /v1/integrations/github/token");
    expect(daemon.routes()).not.toContain("POST /v1/integrations/github/token/test");
  });

  it("saves the token, says so, and shows who GitHub is connected as with the daemon's own test", async () => {
    await openTokenTab();
    paste();
    click(button("Save"));
    await vi.waitFor(() => expect(toasts()).toContain("GitHub connected"));
    expect(daemon.bodies("PUT /v1/integrations/github/token")).toEqual([{ token: TOKEN }]);
    await see("Connected as @ada with a personal access token");
    await vi.waitFor(() => expect(githubRow()?.st).toBe("connected"));
    // The daemon tested it as part of the save, and the dialog shows what the test found.
    expect(has(/A ping reached Marshal/)).toBe(true);
    expect(button("Test connection")).toBeEnabled();
    expect(button("Disconnect")).toBeEnabled();
    // The token is not kept in the field once it is stored: it can be replaced, never read back.
    expect(tokenField()).toHaveValue("");
    expect(within(dialog()).getByText("Replace token")).toBeInTheDocument();
    expect(has("Saving a new token replaces this one.")).toBe(true);
  });

  it("shows the daemon's sentence beside the field when GitHub refuses the token", async () => {
    await openTokenTab();
    daemon.integrations.refuseSave = () => "GitHub did not accept that token.";
    paste("not-a-token");
    click(button("Save"));
    await see("GitHub did not accept that token.");
    expect(githubRow()?.st).toBe("none");
    expect(toasts()).not.toContain("GitHub connected");
    // Typing again takes the old complaint away.
    paste("another");
    expect(has("GitHub did not accept that token.")).toBe(false);
  });

  it("shows a connection made by a token with its stored test and Disconnect", async () => {
    await openTokenTab(true, "token");
    expect(has("Connected as @ada with a personal access token")).toBe(true);
    click(button("Test connection"));
    await vi.waitFor(() => expect(toasts()).toContain("GitHub test passed"));
    expect(daemon.routes()).toContain("POST /v1/integrations/github/test");
    expect(has(/A ping reached Marshal/)).toBe(true);
    click(button("Disconnect"));
    M.S.dialog?.run();
    await vi.waitFor(() => expect(githubRow()?.st).toBe("none"));
    // Disconnected: the tab goes back to an empty field, with nothing about a connection.
    await vi.waitFor(() => expect(has(/Connected as/)).toBe(false));
    expect(within(dialog()).getByText("Personal access token")).toBeInTheDocument();
  });

  it("says a token saved over a sign-in replaces it", async () => {
    await openTokenTab(true);
    expect(has("GitHub is connected by signing in now. Saving a token replaces that.")).toBe(true);
    expect(has(/Connected as/)).toBe(false);
  });

  it("offers the sign-in on the first tab of a token connection, and says it replaces the token", async () => {
    await openDialog(true, "token");
    expect(has("A personal access token is connected now. Signing in replaces it.")).toBe(true);
    expect(has(/Connected as/)).toBe(false);
    expect(button("Sign in with GitHub")).toBeEnabled();
  });
});

describe("a connection's own Test button and fold", () => {
  const inGithub = () => within(githubPanel());
  const testButton = () => inGithub().getByRole("button", { name: "Test" });

  it("shows Test only once something is stored, and runs the stored test from the row", async () => {
    render(() => <SettingsView />);
    expect(inGithub().queryByRole("button", { name: "Test" })).not.toBeInTheDocument();
    cleanup();
    resetIntegrations(true);
    render(() => <SettingsView />);
    click(testButton());
    await vi.waitFor(() => expect(toasts()).toContain("GitHub test passed"));
    expect(daemon.routes()).toContain("POST /v1/integrations/github/test");
    await vi.waitFor(() =>
      expect(inGithub().getByText(/A ping reached Marshal/)).toBeInTheDocument(),
    );
  });

  it("unfolds a folded connection when its test runs, so the result is in view", async () => {
    resetIntegrations(true);
    render(() => <SettingsView />);
    expect(screen.getByRole("button", { name: "Expand GitHub" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    click(testButton());
    await vi.waitFor(() =>
      expect(inGithub().getByText(/A ping reached Marshal/)).toBeInTheDocument(),
    );
    expect(screen.getByRole("button", { name: "Collapse GitHub" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("folds the test result away in the list and in the cards, and keeps Test and Manage", async () => {
    resetIntegrations(true);
    render(() => <SettingsView />);
    click(testButton());
    await vi.waitFor(() =>
      expect(inGithub().getByText(/A ping reached Marshal/)).toBeInTheDocument(),
    );
    for (const view of ["List view", "Card view"]) {
      click(screen.getByRole("button", { name: view }));
      expect(inGithub().getByText(/A ping reached Marshal/)).toBeInTheDocument();
      click(screen.getByRole("button", { name: "Collapse GitHub" }));
      expect(inGithub().queryByText(/A ping reached Marshal/)).not.toBeInTheDocument();
      expect(testButton()).toBeEnabled();
      expect(inGithub().getByRole("button", { name: "Manage" })).toBeEnabled();
      click(screen.getByRole("button", { name: "Expand GitHub" }));
    }
  });
});
