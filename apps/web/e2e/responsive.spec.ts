import { expect, test } from "@playwright/test";
import { openApp, SIZES, scrollsSideways, wrappedButtons } from "./support/app";

type Marshal = NonNullable<Window["M"]>;
type GoArgs = Parameters<Marshal["go"]>;
type CardTab = NonNullable<Parameters<Marshal["openCard"]>[1]>;

/** Every screen a user can reach from the navigation, by the arguments of `M.go`. */
const ROUTES: readonly { name: string; go: GoArgs }[] = [
  { name: "home", go: ["home"] },
  { name: "all activity", go: ["all"] },
  { name: "chats", go: ["project", "api", "chat"] },
  { name: "agents", go: ["project", "api", "agents"] },
  { name: "board", go: ["project", "api", "board"] },
  { name: "list", go: ["project", "api", "list"] },
  { name: "timeline", go: ["project", "api", "timeline"] },
  { name: "calendar", go: ["project", "api", "calendar"] },
  { name: "settings", go: ["settings"] },
];

const CARD_TABS: readonly CardTab[] = [
  "chat",
  "activity",
  "checks",
  "comments",
  "notes",
  "preview",
];
const CARD_NUMBER = 41;
/** A card is known by its project and number. */
const CARD_KEY = `api#${CARD_NUMBER}`;
const MIN_HEIGHT_FOR_CARD_PANEL = 800;

for (const size of SIZES) {
  test.describe(`${size.name} (${size.width} by ${size.height})`, () => {
    test("fills the window, sets its size class, and has no prototype toolbar", async ({
      page,
    }) => {
      const problems = await openApp(page, size);
      const root = page.locator("[data-app-root]");
      await expect(root).toHaveAttribute("data-size", size.name);
      const box = await root.boundingBox();
      expect(box?.width).toBe(size.width);
      expect(box?.height).toBe(size.height);
      await expect(page.getByRole("toolbar", { name: "Prototype controls" })).toHaveCount(0);
      expect(problems.list()).toEqual([]);
    });

    for (const route of ROUTES) {
      test(`${route.name} fits without sideways scrolling or errors`, async ({ page }) => {
        const problems = await openApp(page, size);
        await page.evaluate((args) => window.M?.go(...args), route.go);
        await expect(page.locator("[data-app-root]")).toBeVisible();
        expect(await scrollsSideways(page)).toBe(false);
        expect(await wrappedButtons(page)).toEqual([]);
        expect(problems.list()).toEqual([]);
      });
    }

    for (const tab of CARD_TABS) {
      test(`card #${CARD_NUMBER} ${tab} tab fits without sideways scrolling`, async ({ page }) => {
        const problems = await openApp(page, size);
        await page.evaluate(() => window.M?.go("project", "api", "board"));
        await page.evaluate(([id, name]) => window.M?.openCard(id, name), [CARD_KEY, tab] as const);
        // On a very short phone the card's header fills the screen and leaves the panel no height.
        const panel = page.getByRole("tabpanel");
        if (size.height >= MIN_HEIGHT_FOR_CARD_PANEL) await expect(panel).toBeVisible();
        else await expect(panel).toBeAttached();
        expect(await scrollsSideways(page)).toBe(false);
        expect(problems.list()).toEqual([]);
      });
    }
  });
}

test("follows the window as it is resized between sizes", async ({ page }) => {
  await openApp(page, SIZES[2]);
  const root = page.locator("[data-app-root]");
  await expect(root).toHaveAttribute("data-size", "desktop");
  await expect(page.getByRole("navigation", { name: "Main" })).toBeVisible();

  await page.setViewportSize({ width: SIZES[1].width, height: SIZES[1].height });
  await expect(root).toHaveAttribute("data-size", "tablet");

  await page.setViewportSize({ width: SIZES[0].width, height: SIZES[0].height });
  await expect(root).toHaveAttribute("data-size", "phone");
  await expect(page.locator("[data-tour=views-phone]")).toBeVisible();
  expect(await scrollsSideways(page)).toBe(false);

  await page.setViewportSize({ width: SIZES[2].width, height: SIZES[2].height });
  await expect(root).toHaveAttribute("data-size", "desktop");
  await expect(page.locator("[data-tour=views-phone]")).toHaveCount(0);
});
