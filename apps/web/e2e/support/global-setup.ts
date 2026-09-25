import { readFileSync } from "node:fs";
import { join } from "node:path";
import { DATA_DIR, VITE_PORT } from "./e2e-env";

/**
 * Runs once, after the two servers are up. With `reuseExistingServer` a Vite server that was already
 * listening on the port would be used as it is, and one started for the developer's own dev daemon
 * proxies to it (port 47801) with its real token, so the specs would add and remove projects there.
 * The Vite server of this run reads its token from this run's data folder, so the two tokens must
 * match: if they do not, stop before a single spec runs.
 */
export default async function globalSetup(): Promise<void> {
  const expected = readFileSync(join(DATA_DIR, "dev-token"), "utf8").trim();
  const answer = await fetch(`http://localhost:${VITE_PORT}/__marshal/dev-token`);
  const body: unknown = answer.ok ? await answer.json() : null;
  const served = typeof body === "object" && body !== null ? Reflect.get(body, "token") : null;
  if (served !== expected) {
    throw new Error(
      `The Vite server on port ${VITE_PORT} is not the one this run should use: it does not serve this run's ` +
        "dev token, so it points at another daemon. Stop it and run the tests again.",
    );
  }
}
