import type { SaveGitHubRequest } from "@marshal/protocol";
import { M } from "~/mock";

/**
 * The daemon-side buttons of a connection row in Settings (section S29a). The words belong here,
 * with the screen: the daemon's own refusal is already shown by `optimistic` in its own words, so
 * nothing here judges one. Each answers whether it worked, so the form can close on a save.
 */

/** Stores the GitHub App's connection and says so. The daemon tests it as part of the same call. */
export function connectGitHub(body: SaveGitHubRequest): Promise<boolean> {
  return M.connectGitHub(body).then((saved) => {
    if (saved) M.toast("GitHub connected");
    return saved;
  });
}

/** Runs a connection's own test now and says what it found. */
export function testConnection(id: string, name: string): Promise<void> {
  return M.testIntegration(id).then((ok) => {
    M.toast(ok ? `${name} test passed` : `${name} test failed`);
  });
}

/**
 * Forgets a connection's settings and its secret, after asking. The confirm copy says what is
 * discarded rather than promising a backup: the daemon keeps none.
 */
export function disconnectConnection(id: string, name: string): void {
  M.confirm({
    title: `Disconnect ${name}`,
    message: `This forgets ${name}'s settings and its secret. Marshal stops using it until you connect it again.`,
    action: "Disconnect",
    destructive: true,
    run: () => {
      void M.disconnectIntegration(id).then((done) => {
        if (done) M.toast(`${name} disconnected`);
      });
    },
  });
}
