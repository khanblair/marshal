import { readFileSync } from "node:fs";
import { join } from "node:path";
import { assertBootEndpoints, finishFirstLaunch } from "./daemon-api";
import { DATA_DIR, VITE_ORIGIN } from "./e2e-env";

const TOKEN_REQUEST_TIMEOUT_MS = 10_000;

/**
 * Runs once, after the two servers are up. With `reuseExistingServer` a Vite server that was already
 * listening on the port would be used as it is, and one started for the developer's own dev daemon
 * proxies to it (port 47801) with its real token, so the specs would add and remove projects there.
 * The Vite server of this run reads its token from this run's data folder, so the two tokens must
 * match: if they do not, stop before a single spec runs.
 *
 * The daemon is made new each run, so its first launch is pending and would put the first-launch
 * screens in every spec's way. End it once here, as a person who skipped it does. The one spec that
 * walks those screens puts the first launch back itself.
 */
export default async function globalSetup(): Promise<void> {
  const expected = readFileSync(join(DATA_DIR, "dev-token"), "utf8").trim();
  const tokenURL = `${VITE_ORIGIN}/__marshal/dev-token`;
  const answer = await fetch(tokenURL, {
    signal: AbortSignal.timeout(TOKEN_REQUEST_TIMEOUT_MS),
  }).catch((cause: unknown) => {
    throw new Error(`Could not reach the E2E Vite server at ${VITE_ORIGIN}.`, { cause });
  });
  const body: unknown = answer.ok ? await answer.json() : null;
  const served = typeof body === "object" && body !== null ? Reflect.get(body, "token") : null;
  if (served !== expected) {
    throw new Error(
      `The Vite server at ${VITE_ORIGIN} is not the one this run should use: it does not serve this run's ` +
        "dev token, so it points at another daemon. Stop it and run the tests again.",
    );
  }
  await finishFirstLaunch();
  await assertBootEndpoints();
}
