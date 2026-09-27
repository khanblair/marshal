import { Button } from "@marshal/ui";
import { createSignal } from "solid-js";
import type { Integration } from "~/mock";
import { testConnection } from "./integration-actions";

/**
 * The Obsidian vault connection's own row detail (section S29b, build-plan task 7.7): a "Test
 * connection" button and nothing else. Marshal owns this connection - the vault is its own data
 * folder under `<data>/vault`, not a setting a person fills in - so there is no form to save and
 * nothing to disconnect, only the same test GitHub's form offers once it is connected
 * (`GitHubAppForm`'s "Test connection"). The row above already shows the last test's checks
 * (`IntegrationsSection`'s `TestChecks`), so this panel is only the button that runs another one.
 */
export function ObsidianPanel(props: { integration: Integration }) {
  const [busy, setBusy] = createSignal(false);
  const runTest = (): void => {
    setBusy(true);
    void testConnection(props.integration.id, props.integration.name).finally(() =>
      setBusy(false),
    );
  };
  return (
    <div class="flex flex-wrap gap-2">
      <Button disabled={busy()} onClick={runTest}>
        Test connection
      </Button>
    </div>
  );
}
