import { expect, type Page, test } from "@playwright/test";

/** The three sizes the design defines: phone under 640, tablet 640 to 1199, desktop 1200 up. */
const SIZES = [
  { name: "phone", width: 390, height: 844 },
  { name: "tablet", width: 820, height: 1180 },
  { name: "desktop", width: 1440, height: 900 },
  // The smallest phone and a small tablet, where labels used to wrap and headers overflowed.
  { name: "phone", width: 320, height: 640 },
  { name: "tablet", width: 700, height: 900 },
] as const;

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
const CARD_ID = 41;
const MIN_HEIGHT_FOR_CARD_PANEL = 800;

async function openApp(page: Page, size: (typeof SIZES)[number]): Promise<string[]> {
  const problems: string[] = [];
  page.on("pageerror", (error) => problems.push(`page error: ${error.message}`));
  page.on("console", (message) => {
    if (message.type() === "error") problems.push(`console error: ${message.text()}`);
  });
  await page.setViewportSize({ width: size.width, height: size.height });
  await page.addInitScript(() => window.localStorage.setItem("marshal-proto-onboarded", "1"));
  await page.goto("/#nosim");
  await page.waitForFunction(() => Boolean(window.M?.S?.ready));
  return problems;
}

/** True when the page scrolls sideways, which no size of this app should do. */
const scrollsSideways = (page: Page) =>
  page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);

/**
 * Text of `Button` components (fixed-height, centered) whose label is drawn on more than one
 * line. Rows that are buttons, such as feed items, may wrap on purpose and are not checked.
 */
const wrappedButtons = (page: Page) =>
  page.evaluate(() => {
    const wrapped: string[] = [];
    for (const button of document.querySelectorAll(
      "[data-app-root] button.inline-flex.justify-center",
    )) {
      const walker = document.createTreeWalker(button, NodeFilter.SHOW_TEXT);
      for (let node = walker.nextNode(); node; node = walker.nextNode()) {
        if ((node.textContent ?? "").trim().length < 2) continue;
        const range = document.createRange();
        range.selectNodeContents(node);
        const lines = new Set([...range.getClientRects()].map((rect) => Math.round(rect.top)));
        if (lines.size > 1) {
          wrapped.push((button.textContent ?? "").trim());
          break;
        }
      }
    }
    return wrapped;
  });

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
      expect(problems).toEqual([]);
    });

    for (const route of ROUTES) {
      test(`${route.name} fits without sideways scrolling or errors`, async ({ page }) => {
        const problems = await openApp(page, size);
        await page.evaluate((args) => window.M?.go(...args), route.go);
        await expect(page.locator("[data-app-root]")).toBeVisible();
        expect(await scrollsSideways(page)).toBe(false);
        expect(await wrappedButtons(page)).toEqual([]);
        expect(problems).toEqual([]);
      });
    }

    for (const tab of CARD_TABS) {
      test(`card #${CARD_ID} ${tab} tab fits without sideways scrolling`, async ({ page }) => {
        const problems = await openApp(page, size);
        await page.evaluate(() => window.M?.go("project", "api", "board"));
        await page.evaluate(([id, name]) => window.M?.openCard(id, name), [CARD_ID, tab] as const);
        // On a very short phone the card's header fills the screen and leaves the panel no height.
        const panel = page.getByRole("tabpanel");
        if (size.height >= MIN_HEIGHT_FOR_CARD_PANEL) await expect(panel).toBeVisible();
        else await expect(panel).toBeAttached();
        expect(await scrollsSideways(page)).toBe(false);
        expect(problems).toEqual([]);
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
