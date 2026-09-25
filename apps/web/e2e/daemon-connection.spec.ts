import { expect, type Page, test } from "@playwright/test";
import {
  BROWSER_NETWORK_NOISE,
  expectCleanScreen,
  openApp,
  SIZES,
  type Size,
  THEMES,
  type Theme,
  waitUntilOnline,
} from "./support/app";
import { realToken } from "./support/daemon-api";

const REFUSED = "Sign in again. This device's token is missing or no longer valid.";
const FIXTURE_IDS = ["api", "web", "mobile"];
/** The offline bar is one line on a wide screen and wraps on a phone; neither may grow past this. */
const MAX_BAR_HEIGHT_PX = { phone: 120, other: 60 };
const root = (page: Page) => page.locator("[data-app-root]");

interface Case {
  page: Page;
  size: Size;
  theme: Theme;
}

async function checkLoadsFixture({ page, size, theme }: Case): Promise<void> {
  const problems = await openApp(page, size, { theme });
  const projects = await page.evaluate(() =>
    (window.M?.S.projects ?? []).map((p) => ({ id: p.id, name: p.name })),
  );
  expect(projects.map((p) => p.id)).toEqual(expect.arrayContaining(FIXTURE_IDS));
  expect(projects.map((p) => p.name)).toEqual(
    expect.arrayContaining(["api-gateway", "web-dashboard", "mobile-app"]),
  );
  if (size.name === "phone") {
    await expect(page.locator("[data-tour=projects-phone]")).toBeVisible();
  } else {
    await expect(page.getByRole("button", { name: /^api-gateway: / })).toBeVisible();
  }
  await expectCleanScreen(page, problems, theme);
}

async function checkLostScreen({ page, size, theme }: Case): Promise<void> {
  // A proxy answers 502 with no body when the daemon is down, which the app treats as no answer.
  await page.route("**/v1/health", (route) =>
    route.fulfill({ status: 502, contentType: "text/plain", body: "" }),
  );
  const problems = await openApp(page, size, {
    theme,
    online: false,
    allow: BROWSER_NETWORK_NOISE,
  });
  await expect(root(page)).toHaveAttribute("data-connection", "unreachable");
  await expect(page.getByRole("heading", { name: "Can't reach the daemon" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
  await expectCleanScreen(page, problems, theme);

  const details = page.locator("details");
  await expect(details).not.toHaveAttribute("open", "");
  await details.locator("summary").click();
  await expect(details.locator("pre")).toContainText("unreachable");
  await expectCleanScreen(page, problems, theme);
  expect(await page.locator("body").innerText()).not.toContain(realToken());

  await page.unroute("**/v1/health");
  await page.getByRole("button", { name: "Try again" }).click();
  await waitUntilOnline(page);
  await expect(page.getByRole("heading", { name: "Can't reach the daemon" })).toHaveCount(0);
}

async function checkSignIn({ page, size, theme }: Case): Promise<void> {
  // No dev token from the dev server, as in a production build.
  await page.route("**/__marshal/dev-token", (route) =>
    route.fulfill({ status: 404, contentType: "text/plain", body: "" }),
  );
  const problems = await openApp(page, size, {
    theme,
    online: false,
    allow: BROWSER_NETWORK_NOISE,
  });
  await expect(root(page)).toHaveAttribute("data-connection", "unauthorized");
  await expect(page.getByRole("heading", { name: "Sign in to Marshal" })).toBeVisible();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expectCleanScreen(page, problems, theme);

  await page.getByLabel("Access token").fill("this-is-not-the-token");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toHaveText(REFUSED);
  await expect(page.getByRole("heading", { name: "Sign in to Marshal" })).toBeVisible();
  await expectCleanScreen(page, problems, theme);
  expect(await page.locator("body").innerText()).not.toContain("this-is-not-the-token");

  await page.getByLabel("Access token").fill(realToken());
  await page.getByRole("button", { name: "Sign in" }).click();
  await waitUntilOnline(page);
  await expect(page.getByRole("heading", { name: "Sign in to Marshal" })).toHaveCount(0);
  expect(await page.evaluate(() => window.localStorage.getItem("marshal-token"))).toBe(realToken());
  await expectCleanScreen(page, problems, theme);
}

async function checkOfflineBar({ page, size, theme }: Case): Promise<void> {
  // The daemon answers, but the live connection is closed at once, so the app has data and no stream.
  await page.routeWebSocket(/\/v1\/events/, (socket) => socket.close());
  const problems = await openApp(page, size, {
    theme,
    online: false,
    allow: BROWSER_NETWORK_NOISE,
  });
  await expect(root(page)).toHaveAttribute("data-connection", "reconnecting");
  const bar = page.getByRole("status").filter({ hasText: /You're offline\. Marshal reconnects/ });
  await expect(bar).toBeVisible();
  // The bar is the first thing in the main column: flush with the top and one short strip tall.
  const box = await bar.boundingBox();
  expect(box?.y).toBe(0);
  expect(box?.height).toBeLessThan(MAX_BAR_HEIGHT_PX[size.name === "phone" ? "phone" : "other"]);
  await expect(page.getByRole("banner")).toBeVisible();
  await page.waitForFunction(() => (window.M?.S?.projects.length ?? 0) > 0);
  await expectCleanScreen(page, problems, theme);
}

for (const size of SIZES) {
  for (const theme of THEMES) {
    test.describe(`${size.name} (${size.width} by ${size.height}), ${theme} theme`, () => {
      test("loads the prototype's three projects from the daemon", ({ page }) =>
        checkLoadsFixture({ page, size, theme }));
      test("says it can't reach the daemon, hides the technical text in Details, and recovers when asked", ({
        page,
      }) => checkLostScreen({ page, size, theme }));
      test("asks for the token when it has none, refuses a wrong one in plain words, and opens with the right one", ({
        page,
      }) => checkSignIn({ page, size, theme }));
      test("shows the offline bar over the app while the event stream cannot connect", ({ page }) =>
        checkOfflineBar({ page, size, theme }));
    });
  }
}
