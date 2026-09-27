import { expect, type Page, test } from "@playwright/test";
import { BROWSER_NETWORK_NOISE, expectCleanScreen, openApp, SIZES } from "./support/app";
import { integrationsViaApi } from "./support/daemon-api";

/**
 * The connections in Settings (section S29a), of which GitHub's row is the daemon's. The daemon lists
 * every connection Marshal knows, whether or not it is set up, so the row a person reads is its own
 * answer rather than the design's, and the prototype's "installed on 3 repositories" sentence is
 * gone. Nothing here reaches GitHub: the App is never saved, so no secret is written to a keychain
 * and no spec needs a GitHub account.
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
/** The form's own sentence for a key that is plainly not one, before anything is sent. */
const KEY_REFUSED =
  "Paste the App's private key exactly as GitHub generated it, BEGIN and END lines included.";

/** Opens Settings on the Integrations section. */
async function openIntegrations(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Profile and settings" }).click();
  await page.getByRole("menuitem", { name: "Settings", exact: true }).click();
  await page.getByRole("button", { name: "Integrations" }).click();
}

/** The GitHub row's own panel, so a button is found in that row rather than in another one. */
const githubPanel = (page: Page) =>
  page.locator("[data-app-root] div.border", { has: page.getByText("GitHub", { exact: true }) });

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

  test("opens the App's form, and refuses a key that is not one without storing anything", async ({
    page,
    request,
  }) => {
    const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
    await openIntegrations(page);

    await githubPanel(page).getByRole("button", { name: "Connect" }).click();
    await expect(page.getByLabel("App ID")).toBeVisible();
    await page.getByLabel("App ID").fill("1282340");
    await page.getByLabel("Installation ID").fill("55123907");
    // The webhook secret here is a synthetic string: nothing is sent, so nothing is ever stored.
    await page.getByLabel("Private key").fill("not a key");
    await page.getByLabel("Webhook secret").fill("a-synthetic-webhook-secret");
    await page.getByRole("button", { name: "Save connection" }).click();

    // The form's own guard caught it, so no request went up and the daemon stored nothing.
    await expect(page.getByText(KEY_REFUSED)).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(githubPanel(page).getByText(NOT_CONNECTED)).toBeVisible();
    await expect(page.getByText("GitHub connected")).toHaveCount(0);
    expect((await integrationsViaApi(request)).find((row) => row.id === "github")?.st).toBe("none");

    await expectCleanScreen(page, problems, "light");
  });
});
