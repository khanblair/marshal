import type { IntegrationList, SaveGitHubRequest } from "@marshal/protocol";
import type { Ctx } from "~/mock/context";
import { applyIntegrationList, GITHUB_ID } from "./integrations";

/*
 * The writes the Settings Integrations screen makes on the daemon (section S29a). Every one of them
 * answers with the daemon's whole connection list, which is applied to the store as it arrives. One
 * table serves every connection kind (`docs/architecture.md` section 18), so the same three calls
 * serve GitHub today and the later phases' connections tomorrow.
 *
 * The words belong to the screen: "GitHub connected" is said there. A refusal is already shown by
 * `optimistic` in the daemon's own words, and the store then keeps what it had.
 */

/** Runs one connection write and applies the whole list it answers. False means the daemon refused it. */
async function write(c: Ctx, key: string, request: () => Promise<IntegrationList>): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  try {
    const list = await c.optimistic({
      key,
      apply: () => undefined,
      request,
      rollback: () => undefined,
    });
    applyIntegrationList(c, list);
    return true;
  } catch {
    return false;
  }
}

/**
 * Stores the GitHub App's connection: the App's id, its installation id, its private key, and its
 * webhook secret, which arrive together because no part of it is any use alone. The daemon validates
 * the key, writes it and the secret to the OS keychain, tests the connection, and answers the whole
 * list, so nothing is judged here.
 */
export async function connectGitHub(c: Ctx, body: SaveGitHubRequest): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `integration:${GITHUB_ID}`, () => api.saveIntegration(GITHUB_ID, body));
}

/** Forgets a connection's settings and its secret. One that was never set up is not an error. */
export async function disconnectIntegration(c: Ctx, id: string): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `integration-del:${id}`, () => api.removeIntegration(id));
}

/**
 * Runs one connection's test now and mirrors what it found. The answer is the test's own result, so
 * a passed check list comes back true and one with a failed check, which the daemon answered as a
 * result rather than an error, comes back false and is said in the screen's own words. A test that
 * could not run - including one asked for inside the daemon's cooldown - throws, and its own
 * sentence is shown by `optimistic`.
 *
 * After a test the list is read again, because a test can change a row's state (an error after a
 * failed check) and only the daemon decides that. The re-read is best effort: the test's own result
 * is already on the row if it fails.
 */
export async function testIntegration(c: Ctx, id: string): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  let ok: boolean;
  try {
    const result = await c.optimistic({
      key: `integration-test:${id}`,
      apply: () => undefined,
      request: () => api.testIntegration(id),
      rollback: () => undefined,
    });
    ok = result.ok;
  } catch {
    return false;
  }
  try {
    applyIntegrationList(c, await api.listIntegrations());
  } catch {
    // The test ran; only the re-read failed, so the row keeps the state it had.
  }
  return ok;
}
