// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the GitHub connection is the daemon's).
import { daemon } from "~/testing/daemon-integrations-store";
import type { SaveGitHubRequest, TestCheck } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { unwrap } from "solid-js/store";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { golden } from "~/data/testing/golden";
import { M } from "~/mock";
import { createIntegrationStore } from "~/testing/fake-integrations";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/** The App's setup the golden request carries, which is what a save sends up. */
const SAVE = golden<SaveGitHubRequest>("save-github-request");
/** The prototype's own sentence for the GitHub row, which nothing on the daemon ever answers. */
const PROTOTYPE_DETAIL = "GitHub App installed on 3 repositories";
/** The sentence the daemon's own passing test writes into its row (`integrations/test.go`). */
const PASSED_DETAIL = "The GitHub App works and Marshal can use it.";
const FIX = "Grant pull request access to the App on GitHub.";

/** The store's own rows as the seed made them, so a reset can put back the app's words. */
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
 * save. `connected` seeds the GitHub row as set up, which is what shows the Manage form with its
 * test and disconnect buttons.
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

beforeEach(() => resetIntegrations());
afterEach(cleanup);

const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);
const githubRow = () => M.S.integrations.find((integration) => integration.id === "github");
/** The GitHub row's own panel, so a button is found in that row rather than in another one. */
const githubPanel = (): HTMLElement => {
  const panel = screen.getByText("GitHub").closest("div.border");
  if (!(panel instanceof HTMLElement)) throw new Error("the GitHub row is not drawn");
  return panel;
};
const rowButton = (name: string): HTMLElement =>
  within(githubPanel()).getByRole("button", { name });
const click = (button: HTMLElement): void => {
  fireEvent.click(button);
};
const type = (label: string, value: string): void => {
  // A field's own hint sits inside its label, so the label's text is more than the word itself.
  fireEvent.input(screen.getByLabelText(label, { exact: false }), { target: { value } });
};

/** Fills the four fields the App's setup is made of, with the golden values and no error. */
function fillAppForm(overrides: Partial<SaveGitHubRequest> = {}): void {
  const values = { ...SAVE, ...overrides };
  type("App ID", String(values.appId));
  type("Installation ID", String(values.installationId));
  type("Private key", values.privateKey);
  type("Webhook secret", values.webhookSecret);
}

describe("the Integrations section on the daemon", () => {
  it("shows every connection the daemon knows, with the daemon's own state for GitHub", () => {
    render(() => <SettingsView />);
    // The six of later phases are listed and read the way the daemon's own `known()` reports them
    // and the way their own mock rows already read: only GitHub's row is the daemon's today.
    for (const name of ["GitHub", "Trello", "Google Calendar", "Gmail", "Telegram", "Discord"]) {
      expect(screen.getByText(name)).toBeInTheDocument();
    }
    expect(githubRow()?.st).toBe("none");
    // Nothing is stored for GitHub, so the prototype's own "installed" sentence does not show.
    expect(screen.queryByText(PROTOTYPE_DETAIL)).not.toBeInTheDocument();
    expect(rowButton("Connect")).toBeInTheDocument();
    // A later phase's row keeps the prototype's own state until its own phase builds it.
    expect(screen.getAllByRole("button", { name: "Manage" }).length).toBeGreaterThan(0);
    // The daemon was asked for the list while the app came online.
    expect(boot).toContain("GET /v1/integrations");
  });

  it("opens the App's form, and refuses a key that is not one without calling the daemon", () => {
    render(() => <SettingsView />);
    click(rowButton("Connect"));
    expect(screen.getByLabelText("App ID", { exact: false })).toBeInTheDocument();
    fillAppForm({ privateKey: "not a key" });
    click(screen.getByRole("button", { name: "Save connection" }));
    expect(
      screen.getByText(
        "Paste the App's private key exactly as GitHub generated it, BEGIN and END lines included.",
      ),
    ).toBeInTheDocument();
    // Nothing was sent: the form's own guard caught it before a keychain write.
    expect(daemon.bodies("PUT /v1/integrations/github")).toEqual([]);
  });

  it("refuses an id that is not a number, with a sentence that names the mistake", () => {
    render(() => <SettingsView />);
    click(rowButton("Connect"));
    fillAppForm({ installationId: 0 });
    click(screen.getByRole("button", { name: "Save connection" }));
    expect(
      screen.getByText(
        "The App id and the installation id are both numbers. Copy each one from the App's page on GitHub.",
      ),
    ).toBeInTheDocument();
    expect(daemon.bodies("PUT /v1/integrations/github")).toEqual([]);
  });

  it("saves the App's setup on the daemon, says so, and shows the checks it answered", async () => {
    render(() => <SettingsView />);
    click(rowButton("Connect"));
    fillAppForm();
    click(screen.getByRole("button", { name: "Save connection" }));
    await waitFor(() => expect(toasts()).toContain("GitHub connected"));
    // The whole setup goes up together, key and secret and all: no part of it is any use alone.
    expect(daemon.bodies("PUT /v1/integrations/github")).toEqual([SAVE]);
    await waitFor(() => expect(githubRow()?.st).toBe("connected"));
    expect(screen.getByText(PASSED_DETAIL)).toBeInTheDocument();
    // The form closed, so the row's button is a Manage one now.
    expect(screen.queryByLabelText("App ID", { exact: false })).not.toBeInTheDocument();
    expect(rowButton("Manage")).toBeInTheDocument();
    // Every check the daemon's own test looked at is drawn, as a provider row draws its own.
    expect(screen.getByText(/The GitHub App is installed/)).toBeInTheDocument();
    expect(screen.getByText(/3 repositories are visible/)).toBeInTheDocument();
    expect(screen.getByText(/A ping reached Marshal/)).toBeInTheDocument();
  });

  it("runs the connection's test from the form and says what it found", async () => {
    resetIntegrations(true);
    render(() => <SettingsView />);
    click(rowButton("Manage"));
    click(screen.getByRole("button", { name: "Test connection" }));
    await waitFor(() => expect(toasts()).toContain("GitHub test passed"));
    expect(
      daemon.calls.some(
        (call) => call.method === "POST" && call.url.endsWith("/v1/integrations/github/test"),
      ),
    ).toBe(true);
    expect(screen.getByText(/A ping reached Marshal/)).toBeInTheDocument();
  });

  it("marks the row as needing attention when a test finds a bad permission, and shows the fix", async () => {
    resetIntegrations(true);
    daemon.integrations.checks = (): TestCheck[] => [
      {
        name: "Permissions",
        state: "failed",
        message: "The App cannot read pull requests.",
        fix: "Grant pull request access to the App on GitHub.",
      },
    ];
    render(() => <SettingsView />);
    click(rowButton("Manage"));
    click(screen.getByRole("button", { name: "Test connection" }));
    await waitFor(() => expect(toasts()).toContain("GitHub test failed"));
    await waitFor(() => expect(screen.getByText(FIX)).toBeInTheDocument());
    expect(githubRow()?.st).toBe("error");
    // An error row offers Reconnect rather than Manage, so a person is invited back to the form.
    expect(rowButton("Reconnect")).toBeInTheDocument();
  });

  it("forgets the connection after asking, and shows it as not connected again", async () => {
    resetIntegrations(true);
    render(() => <SettingsView />);
    click(rowButton("Manage"));
    click(screen.getByRole("button", { name: "Disconnect" }));
    // The confirm says what is discarded rather than promising a backup: the daemon keeps none.
    expect(M.S.dialog?.title).toBe("Disconnect GitHub");
    M.S.dialog?.run();
    await waitFor(() => expect(toasts()).toContain("GitHub disconnected"));
    expect(daemon.routes()).toContain("DELETE /v1/integrations/github");
    await waitFor(() => expect(githubRow()?.st).toBe("none"));
    expect(screen.queryByText(PROTOTYPE_DETAIL)).not.toBeInTheDocument();
    expect(rowButton("Connect")).toBeInTheDocument();
  });
});
