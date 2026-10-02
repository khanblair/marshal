import { Button } from "@marshal/ui";
import { createSignal, type JSX, Show } from "solid-js";
import type { Integration } from "~/mock";
import { disconnectConnection, testConnection } from "./integration-actions";
import { TestChecks } from "./TestChecks";

/**
 * What a connected GitHub offers on either tab: the stored connection's own test, shown with the
 * checks the daemon's test found, and Disconnect. `children` are the buttons that go before them.
 */
export function GitHubStored(props: {
  integration: Integration;
  /** Called once the daemon has forgotten the connection. */
  onDisconnected: () => void;
  children?: JSX.Element;
}) {
  const [busy, setBusy] = createSignal(false);
  const name = () => props.integration.name;
  const runTest = (): void => {
    setBusy(true);
    void testConnection(props.integration.id, name()).finally(() => setBusy(false));
  };
  return (
    <>
      <div class="flex flex-wrap gap-2">
        {props.children}
        <Button disabled={busy()} onClick={runTest}>
          Test connection
        </Button>
        <Button
          variant="destructive"
          disabled={busy()}
          onClick={() => disconnectConnection(props.integration.id, name(), props.onDisconnected)}
        >
          Disconnect
        </Button>
      </div>
      <Show when={props.integration.st === "error" && props.integration.detail}>
        {(detail) => (
          <span role="alert" class="text-small leading-4.5 text-status-danger-text">
            {detail()}
          </span>
        )}
      </Show>
      <Show when={props.integration.lastTest}>{(test) => <TestChecks test={test()} />}</Show>
    </>
  );
}
