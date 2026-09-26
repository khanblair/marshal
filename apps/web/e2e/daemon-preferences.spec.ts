import { expect, type Page, test } from "@playwright/test";
import {
  BROWSER_NETWORK_NOISE,
  expectCleanScreen,
  openApp,
  SIZES,
  waitUntilOnline,
} from "./support/app";
import { createProjectViaApi, meViaApi, removeProjectViaApi } from "./support/daemon-api";
import { makeRepo } from "./support/git";

/**
 * The two things a person sets and expects back after closing the app: the colour scheme, and a view
 * of a board saved under a name. Both are the daemon's (section S32 and S6a), so each one is checked
 * on the daemon and again after a reload.
 */
const DESKTOP = SIZES[2];
const RADIX = 36;
const RANDOM_START = 2;
const RANDOM_END = 8;

const uniqueName = (label: string): string =>
  `e2e-${label}-${Math.random().toString(RADIX).slice(RANDOM_START, RANDOM_END)}`;

/** The theme cards' own labels for the three values the account can hold. */
const themeLabel = (theme: string): string =>
  ({ light: "Light", dark: "Dark", system: "System" })[theme] ?? "System";

/** Opens the Settings section from the profile menu. */
async function openSettings(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Profile and settings" }).click();
  await page.getByRole("menuitem", { name: "Settings", exact: true }).click();
  await expect(page.getByRole("radiogroup", { name: "Theme" })).toBeVisible();
}

test.describe("a preference", () => {
  test("keeps the theme the person picked, over a reload", async ({ page, request }) => {
    const before = (await meViaApi(request)).preferences.theme;
    const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
    await openSettings(page);

    // The cards write the choice straight into the store, and the account saves it.
    await page.getByRole("radio", { name: "Dark" }).click();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    await expect.poll(async () => (await meViaApi(request)).preferences.theme).toBe("dark");

    // The daemon has it, so a fresh load comes up dark without the person asking again.
    await page.reload();
    await waitUntilOnline(page);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    await openSettings(page);
    await expect(page.getByRole("radio", { name: "Dark" })).toBeChecked();
    await expectCleanScreen(page, problems, "dark");

    // Put back what the run started on, so the shared daemon is left as the next spec finds it.
    await page.getByRole("radio", { name: themeLabel(before) }).click();
    await expect.poll(async () => (await meViaApi(request)).preferences.theme).toBe(before);
  });
});

test.describe("a saved view", () => {
  test("saves the filters under a name, and opens that view again after a reload", async ({
    page,
    request,
  }) => {
    const repo = makeRepo("e2e-views");
    const name = uniqueName("views");
    let id = "";
    try {
      const project = await createProjectViaApi(request, repo.dir, name);
      id = project.id;

      const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
      await page.evaluate((pid) => window.M?.go("project", pid, "board"), id);
      await expect(page.getByText("This board has no cards yet.")).toBeVisible();

      // The view starts unsaved, and the menu takes a name for what is on screen.
      await page.getByRole("button", { name: "Unsaved view" }).click();
      await page.getByLabel("Name this view").fill("Mine");
      await page.getByRole("button", { name: "Save view" }).click();
      await expect(page.getByText("View saved")).toBeVisible();
      await expect(page.getByRole("button", { name: "Mine" })).toBeVisible();

      // The view is the daemon's, so a reload lists it and opens on it.
      await page.reload();
      await waitUntilOnline(page);
      await page.evaluate((pid) => window.M?.go("project", pid, "board"), id);
      await expect(page.getByRole("button", { name: "Mine" })).toBeVisible();
      await page.getByRole("button", { name: "Mine" }).click();
      await expect(page.getByRole("menuitemradio", { name: /Mine/ })).toHaveAttribute(
        "aria-checked",
        "true",
      );

      await expectCleanScreen(page, problems, "light");
    } finally {
      if (id) await removeProjectViaApi(request, id);
    }
  });
});
