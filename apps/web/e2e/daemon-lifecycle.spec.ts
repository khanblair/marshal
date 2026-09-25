import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import { BROWSER_NETWORK_NOISE, expectCleanScreen, openApp, SIZES } from "./support/app";
import { DAEMON_ORIGIN, RUNNER_PID_FILE } from "./support/e2e-env";

/*
 * These specs stop and start the real daemon, so they run alone, after every other spec (see the
 * `lifecycle` project in playwright.config.ts). They signal the script that owns the daemon
 * (`scripts/e2e-daemon.mjs`), never the daemon itself, and there is no control endpoint.
 */

const CHANGE_TIMEOUT_MS = 45_000;
const FIXTURE_PROJECT_COUNT = 3;
const runnerPid = (): number => Number(readFileSync(RUNNER_PID_FILE, "utf8"));

async function healthy(): Promise<boolean> {
  try {
    return (await fetch(`${DAEMON_ORIGIN}/v1/health`)).ok;
  } catch {
    return false;
  }
}

async function stopDaemon(): Promise<void> {
  process.kill(runnerPid(), "SIGUSR2");
  await expect.poll(healthy, { timeout: CHANGE_TIMEOUT_MS }).toBe(false);
}

async function startDaemon(): Promise<void> {
  process.kill(runnerPid(), "SIGHUP");
  await expect.poll(healthy, { timeout: CHANGE_TIMEOUT_MS }).toBe(true);
}

// Whatever a spec does, the daemon must be running for the next one.
test.afterEach(async () => {
  if (!(await healthy())) await startDaemon();
});

const CASES = [
  { size: SIZES[2], theme: "light" },
  { size: SIZES[0], theme: "dark" },
] as const;

for (const { size, theme } of CASES) {
  test(`shows the lost screen when the daemon stops and the app again, with no reload, when it returns (${size.name}, ${theme})`, async ({
    page,
  }) => {
    const problems = await openApp(page, size, { theme, allow: BROWSER_NETWORK_NOISE });
    const root = page.locator("[data-app-root]");
    // A value the page keeps only until it reloads, to prove that it never did.
    await page.evaluate(() => Reflect.set(window, "e2eSurvivor", 1));

    await stopDaemon();
    await expect(root).toHaveAttribute("data-connection", "unreachable", {
      timeout: CHANGE_TIMEOUT_MS,
    });
    await expect(page.getByRole("heading", { name: "Can't reach the daemon" })).toBeVisible();
    await expectCleanScreen(page, problems, theme);

    await startDaemon();
    await expect(root).toHaveAttribute("data-connection", "online", { timeout: CHANGE_TIMEOUT_MS });
    await expect(page.getByRole("heading", { name: "Can't reach the daemon" })).toHaveCount(0);
    await page.waitForFunction(
      (count) => (window.M?.S?.projects.length ?? 0) >= count,
      FIXTURE_PROJECT_COUNT,
    );
    if (size.name === "phone") {
      await expect(page.locator("[data-tour=projects-phone]")).toBeVisible();
    } else {
      await expect(page.getByRole("button", { name: /^api-gateway: / })).toBeVisible();
    }
    expect(await page.evaluate(() => Reflect.get(window, "e2eSurvivor"))).toBe(1);
    await expectCleanScreen(page, problems, theme);
  });
}
