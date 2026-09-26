import { expect, type Page, test } from "@playwright/test";
import {
  BROWSER_NETWORK_NOISE,
  expectCleanScreen,
  openApp,
  SIZES,
  waitUntilOnline,
} from "./support/app";
import { askForSampleViaApi, projectsViaApi, resetFirstLaunch } from "./support/daemon-api";

/**
 * The first-launch screens, the tour, and the sample project. The run's daemon is made new each
 * time, so its first launch is pending until something ends it; `global-setup` ends it once so the
 * other specs start inside the app. This is the one spec that puts it back, which is why it runs
 * last, alone (see the `onboarding` project in playwright.config.ts).
 */
const DESKTOP = SIZES[2];
const SAMPLE = "marshal-sample";
const ALREADY_THERE = "The sample project is already in Marshal.";
const NAME = "Ada Lovelace";
/** The first launch's five screens, as the design draws them. */
const SCREENS = 5;

const heading = (page: Page, title: string) => page.getByRole("heading", { name: title });
const stepLabel = (page: Page, step: number) => page.getByText(`Step ${step} of ${SCREENS}`);
const continueButton = (page: Page) => page.getByRole("button", { name: "Continue" });
/** The tour's card; the design's first stop is the tiles on Home. */
const tour = (page: Page) => page.getByRole("dialog", { name: "Summary and charts" });

/** What the app has raised, as the store holds it. */
const toasts = (page: Page): Promise<string[]> =>
  page.evaluate(() => (window.M?.S.toasts ?? []).map((toast) => toast.msg));

/** Walks the screens to the project one, naming the person on the way, as the second screen asks. */
async function walkToProjectStep(page: Page): Promise<void> {
  await continueButton(page).click();
  await page.getByLabel("Name", { exact: true }).fill(NAME);
  await continueButton(page).click();
  await continueButton(page).click();
  await expect(heading(page, "Add your first project")).toBeVisible();
}

test.beforeEach(async () => {
  await resetFirstLaunch();
});

test("shows the first-launch screens and opens again at the saved step", async ({ page }) => {
  const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
  await expect(heading(page, "Welcome to Marshal")).toBeVisible();
  await expect(stepLabel(page, 1)).toBeVisible();

  await continueButton(page).click();
  await expect(heading(page, "Set up your profile")).toBeVisible();
  await expect(stepLabel(page, 2)).toBeVisible();

  // The screen is the daemon's, so a reload opens the same one rather than the first.
  await page.reload();
  await waitUntilOnline(page);
  await expect(heading(page, "Set up your profile")).toBeVisible();
  await expect(stepLabel(page, 2)).toBeVisible();

  // Back moves one screen, and that is saved too.
  await page.getByRole("button", { name: "Back" }).click();
  await expect(heading(page, "Welcome to Marshal")).toBeVisible();
  await expect(stepLabel(page, 1)).toBeVisible();
  await expectCleanScreen(page, problems);
});

test("skips to the end, runs the tour, and replays it from the profile menu", async ({ page }) => {
  const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
  await expect(heading(page, "Welcome to Marshal")).toBeVisible();

  // Skip moves to the next screen, and on the last screen it ends the first launch, as the design does.
  const skip = page.getByRole("button", { name: "Skip", exact: true });
  for (let screen = 0; screen < SCREENS; screen += 1) await skip.click();
  await expect(page.getByRole("dialog", { name: "Welcome to Marshal" })).toHaveCount(0);

  // Both Skip and the end open the tour, which covers Home until it is put away.
  await expect(tour(page)).toBeVisible();
  await page.getByRole("button", { name: "Skip tour" }).click();
  await expect(tour(page)).toHaveCount(0);

  // The profile menu replays it, and Skip puts it away again.
  await page.getByRole("button", { name: "Profile and settings" }).click();
  await page.getByRole("menuitem", { name: "Replay tour" }).click();
  await expect(tour(page)).toBeVisible();
  await page.getByRole("button", { name: "Skip tour" }).click();
  await expect(tour(page)).toHaveCount(0);
  await expectCleanScreen(page, problems);
});

test("adds the sample project, and refuses a second one in the daemon's own words", async ({
  page,
  request,
}) => {
  const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
  await walkToProjectStep(page);
  // The design starts this screen on the sample, the one choice that needs nothing typed.
  await expect(
    page.getByRole("radiogroup", { name: "How to add your first project" }),
  ).toBeVisible();
  await expect(page.getByText("The sample project is safe to try things on.")).toBeVisible();

  await continueButton(page).click();
  await expect
    .poll(async () => (await projectsViaApi(request)).filter((p) => p.name === SAMPLE).length)
    .toBe(1);

  // The last screen's Continue ends the first launch, opens the tour, and shows Home.
  await page.getByRole("button", { name: "Open Marshal" }).click();
  await page.getByRole("button", { name: "Skip tour" }).click();
  await expect(page.getByRole("button", { name: new RegExp(`^${SAMPLE}: `) })).toBeVisible();

  // Go round again with the sample already in Marshal: the daemon's sentence, and no second project.
  await resetFirstLaunch();
  await page.reload();
  await waitUntilOnline(page);
  await walkToProjectStep(page);
  await continueButton(page).click();
  await expect.poll(() => toasts(page)).toContain(ALREADY_THERE);
  expect((await projectsViaApi(request)).filter((p) => p.name === SAMPLE)).toHaveLength(1);

  // The sentence on screen is the daemon's own, word for word, and it is still only the one sample.
  const refused = await askForSampleViaApi(request);
  expect(refused.ok).toBe(false);
  expect(refused.message).toBe(ALREADY_THERE);
  await expectCleanScreen(page, problems);
});
