import { expect, type Page, test } from "@playwright/test";
import { BROWSER_NETWORK_NOISE, expectCleanScreen, openApp, SIZES } from "./support/app";
import { integrationsViaApi } from "./support/daemon-api";

/**
 * The connections in Settings (section S29a), of which GitHub's row is the daemon's. The daemon lists
 * every connection Marshal knows, whether or not it is set up, so the row a person reads is its own
 * answer rather than the design's, and the prototype's "installed on 3 repositories" sentence is
 * gone. Nothing here reaches GitHub: no sign-in is started and no token is tested or saved, so no
 * secret is written to a keychain and no spec needs a GitHub account.
 */
const DESKTOP = SIZES[2];
/** The connections the daemon knows, in its own order (`integrations.go`'s `known()`). */
const CONNECTIONS = [
  "GitHub",
  "Trello",
  "Google Calendar",
  "Gmail",
  "Telegram",
  "Discord",
  "Obsidian",
];
/** The prototype's own sentence for the GitHub row, which nothing on the daemon answers. */
const PROTOTYPE_DETAIL = "GitHub App installed on 3 repositories";
const NOT_CONNECTED = "Not connected";
const SIGN_IN_SENTENCE =
  "Sign in with your GitHub account. Marshal opens GitHub, shows a code, and does the rest.";
const TOKEN_HELP =
  "Use a classic token with the repo scope, or a fine-grained token with read and write access to Contents, Pull requests, Issues and Actions, and read access to Checks. Marshal keeps it in the OS keychain.";
/** The dialog's own sentence for an empty token field, before anything is sent. */
const TOKEN_MISSING = "Paste a GitHub token first.";

/** Opens Settings on the Integrations section. */
async function openIntegrations(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Profile and settings" }).click();
  await page.getByRole("menuitem", { name: "Settings", exact: true }).click();
  await page.getByRole("button", { name: "Integrations" }).click();
}

/** The GitHub row's own panel, so a button is found in that row rather than in another one. */
const githubPanel = (page: Page) => page.locator('[data-app-root] [data-integration="github"]');

test.describe("the connections the daemon owns", () => {
  test("lists the daemon's connections, and GitHub reads the daemon's own state", async ({
    page,
    request,
  }) => {
    const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
    await openIntegrations(page);

    for (const name of CONNECTIONS) {
      await expect(page.getByText(name, { exact: true })).toBeVisible();
    }
    // The daemon has no GitHub App stored, so its row is not connected and carries no sentence: the
    // prototype's own "installed" sentence is the design's, and the daemon never answers it.
    const github = githubPanel(page);
    await expect(github.getByText(NOT_CONNECTED)).toBeVisible();
    await expect(github.getByRole("button", { name: "Connect" })).toBeVisible();
    await expect(page.getByText(PROTOTYPE_DETAIL)).toHaveCount(0);

    // And the daemon says the same thing it drew, so the row is its answer and not a leftover.
    const wire = await integrationsViaApi(request);
    expect(wire.map((row) => row.id)).toEqual([
      "github",
      "trello",
      "gcal",
      "gmail",
      "telegram",
      "discord",
      "obsidian",
    ]);
    expect(wire.find((row) => row.id === "github")).toMatchObject({ st: "none", detail: "" });

    await expectCleanScreen(page, problems, "light");
  });

  test("opens the GitHub dialog with both ways to connect, and starts nothing", async ({
    page,
    request,
  }) => {
    const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
    await openIntegrations(page);

    await githubPanel(page).getByRole("button", { name: "Connect" }).click();
    const dialog = page.getByRole("dialog", { name: "Connect GitHub" });
    await expect(dialog).toBeVisible();
    // The sign-in is the first tab. Its button is never pressed here: that would ask GitHub for a code.
    await expect(dialog.getByRole("tab", { name: "Sign in with GitHub" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await expect(dialog.getByText(SIGN_IN_SENTENCE)).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Sign in with GitHub" })).toBeEnabled();

    await dialog.getByRole("tab", { name: "Paste a token" }).click();
    await expect(dialog.getByText(TOKEN_HELP)).toBeVisible();
    // The dialog's own guard refuses an empty field, so nothing is sent and nothing is stored.
    await dialog.getByRole("button", { name: "Save" }).click();
    await expect(dialog.getByText(TOKEN_MISSING)).toBeVisible();

    await dialog.getByRole("button", { name: "Close" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(githubPanel(page).getByText(NOT_CONNECTED)).toBeVisible();
    await expect(page.getByText("GitHub connected")).toHaveCount(0);
    expect((await integrationsViaApi(request)).find((row) => row.id === "github")?.st).toBe("none");

    await expectCleanScreen(page, problems, "light");
  });
});
