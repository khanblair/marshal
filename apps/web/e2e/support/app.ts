import { expect, type Page } from "@playwright/test";

/** The three sizes the design defines: phone under 640, tablet 640 to 1199, desktop 1200 up. */
export const SIZES = [
  { name: "phone", width: 390, height: 844 },
  { name: "tablet", width: 820, height: 1180 },
  { name: "desktop", width: 1440, height: 900 },
  // The smallest phone and a small tablet, where labels used to wrap and headers overflowed.
  { name: "phone", width: 320, height: 640 },
  { name: "tablet", width: 700, height: 900 },
] as const;

export type Size = (typeof SIZES)[number];

export const THEMES = ["light", "dark"] as const;
export type Theme = (typeof THEMES)[number];

/**
 * Messages Chrome itself writes to the console when a request fails, whatever the page does with the
 * failure. A spec that makes the daemon refuse or vanish on purpose allows these, and nothing else.
 */
export const BROWSER_NETWORK_NOISE: readonly RegExp[] = [
  /^console error: Failed to load resource/,
  /^console error: WebSocket connection to .* failed/,
];

interface OpenOptions {
  theme?: Theme;
  /** Console messages a spec allows, as regular expressions on `console error: <text>`. */
  allow?: readonly RegExp[];
  /** The page's address after the origin. `#nosim` keeps the mock's simulation off. */
  url?: string;
  /** Waits for the app to be online with its first data. False when the spec expects another screen. */
  online?: boolean;
}

export interface Problems {
  /** What went wrong on the page so far, minus what the spec allowed. Empty when all is well. */
  list(): string[];
}

/** Starts collecting page errors and console errors. Call it before the page loads. */
function watchProblems(page: Page, allow: readonly RegExp[] = []): Problems {
  const seen: string[] = [];
  page.on("pageerror", (error) => seen.push(`page error: ${error.message}`));
  page.on("console", (message) => {
    if (message.type() === "error") seen.push(`console error: ${message.text()}`);
  });
  return { list: () => seen.filter((line) => !allow.some((pattern) => pattern.test(line))) };
}

/**
 * Opens the app at a size and theme, and waits until it is online with the daemon's data
 * (`data-connection="online"` and `M.S.ready`). Returns what to check for console errors.
 *
 * The first launch is the daemon's to remember now, and `global-setup` ended it once for the whole
 * run, so a spec lands on those screens only when it puts the first launch back itself.
 */
export async function openApp(
  page: Page,
  size: { width: number; height: number },
  options: OpenOptions = {},
): Promise<Problems> {
  const problems = watchProblems(page, options.allow);
  await page.setViewportSize({ width: size.width, height: size.height });
  await page.emulateMedia({ colorScheme: options.theme ?? "light" });
  await page.goto(options.url ?? "/#nosim");
  if (options.online === false) {
    await page.waitForFunction(() => Boolean(window.M?.S?.ready));
    return problems;
  }
  await waitUntilOnline(page);
  return problems;
}

/** The app has its first data from the daemon and shows itself. */
export async function waitUntilOnline(page: Page): Promise<void> {
  await page.waitForFunction(() => Boolean(window.M?.S?.ready));
  await expect(page.locator("[data-app-root]")).toHaveAttribute("data-connection", "online");
  await page.waitForFunction(() => (window.M?.S?.projects.length ?? 0) > 0);
}

/** True when the page scrolls sideways, which no size of this app should do. */
export const scrollsSideways = (page: Page): Promise<boolean> =>
  page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);

/**
 * Text of `Button` components (fixed-height, centered) whose label is drawn on more than one
 * line. Rows that are buttons, such as feed items, may wrap on purpose and are not checked.
 */
export const wrappedButtons = (page: Page): Promise<string[]> =>
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

/**
 * What every screen of this app must satisfy at every size and in both themes: no sideways scroll, no
 * button label on two lines, and nothing in the console. `theme` is checked against the page too.
 */
export async function expectCleanScreen(
  page: Page,
  problems: Problems,
  theme?: Theme,
): Promise<void> {
  expect(await scrollsSideways(page)).toBe(false);
  expect(await wrappedButtons(page)).toEqual([]);
  if (theme) await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
  expect(problems.list()).toEqual([]);
}
