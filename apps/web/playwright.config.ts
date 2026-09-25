import { defineConfig } from "@playwright/test";
import { DAEMON_ORIGIN, DAEMON_PORT, DATA_DIR, VITE_PORT } from "./e2e/support/e2e-env";

/** The one spec that stops and starts the daemon, so it must not run beside the others. */
const LIFECYCLE_SPEC = /daemon-lifecycle\.spec\.ts/;
const SERVER_START_MS = 60_000;
const DAEMON_STOP_MS = 10_000;

/**
 * End-to-end checks run against two servers of their own: the built dev daemon (`pnpm build` first)
 * on its own port with a throwaway data folder, and a Vite dev server that forwards `/v1` to it. They
 * never touch the developer's dev daemon (port 47801) or port 3210.
 */
export default defineConfig({
  testDir: "e2e",
  fullyParallel: true,
  reporter: "list",
  // Playwright empties its output folder; the daemon's data folder lives beside it, not in it.
  outputDir: "test-results/playwright",
  // Stops the run early when a server that was already running is not the one made for it.
  globalSetup: "./e2e/support/global-setup.ts",
  use: { baseURL: `http://localhost:${VITE_PORT}` },
  projects: [
    { name: "app", testIgnore: LIFECYCLE_SPEC },
    // Stopping the daemon would break every other spec that is running, so this one runs alone, after them.
    { name: "lifecycle", testMatch: LIFECYCLE_SPEC, dependencies: ["app"], fullyParallel: false },
  ],
  webServer: [
    {
      command: "node ../../scripts/e2e-daemon.mjs",
      url: `${DAEMON_ORIGIN}/v1/health`,
      reuseExistingServer: !process.env.CI,
      timeout: SERVER_START_MS,
      gracefulShutdown: { signal: "SIGTERM", timeout: DAEMON_STOP_MS },
      env: { E2E_DAEMON_PORT: String(DAEMON_PORT), E2E_DATA_DIR: DATA_DIR },
    },
    {
      command: `pnpm exec vite --port ${VITE_PORT} --strictPort`,
      url: `http://localhost:${VITE_PORT}`,
      reuseExistingServer: !process.env.CI,
      // The dev server forwards `/v1` to this daemon and reads its dev token from this folder.
      env: { MARSHAL_PORT: String(DAEMON_PORT), MARSHAL_DATA_DIR: DATA_DIR },
    },
  ],
});
