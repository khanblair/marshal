import { Button, Field, Input } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import type { Integration } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";
import { connectNtfy, disconnectConnection, testConnection } from "./integration-actions";

/**
 * ntfy's whole setup (B9.3, section S29h): the topic alerts are published to, and optionally the
 * server and an access token. Unlike a chat bot it needs no token on ntfy's own public server, so
 * only the topic is required. The daemon publishes a test message as part of the same call, and the
 * ntfy app on the phone shows alerts as ordinary notifications, even while Marshal is closed.
 */
export function NtfyForm(props: { integration: Integration; edit: EditState }) {
  const error = () => props.edit.errorFor(props.integration.id);
  const connected = () => props.integration.st !== "none";
  const [busy, setBusy] = createSignal(false);
  const name = () => props.integration.name;

  const submit = (form: HTMLFormElement): void => {
    const topic = fieldValue(form, "topic").trim();
    if (!topic) {
      props.edit.fail("Enter the ntfy topic Marshal should send alerts to.");
      return;
    }
    setBusy(true);
    void connectNtfy({
      topic,
      server: fieldValue(form, "server").trim(),
      token: fieldValue(form, "token").trim(),
    })
      .then((saved) => {
        if (saved) props.edit.close();
      })
      .finally(() => setBusy(false));
  };

  const runTest = (): void => {
    setBusy(true);
    void testConnection(props.integration.id, name()).finally(() => setBusy(false));
  };

  return (
    <form
      class="flex flex-col gap-2"
      onSubmit={(event) => {
        event.preventDefault();
        submit(event.currentTarget);
      }}
    >
      <Field
        label="Topic"
        hint="Pick a long, hard-to-guess name. On a public server anyone who knows it can read your alerts."
      >
        <Input mono name="topic" autocomplete="off" invalid={!!error()} />
      </Field>
      <Field label="Server" hint="Leave empty for ntfy.sh, or give the address of your own.">
        <Input mono name="server" autocomplete="off" placeholder="https://ntfy.sh" />
      </Field>
      <Field
        label="Access token"
        hint="Only for a server that needs one. Marshal keeps it in the OS keychain."
      >
        <Input type="password" name="token" autocomplete="off" />
      </Field>
      <Show when={error()}>
        <span class="text-small leading-4.5 text-status-danger-text">{error()}</span>
      </Show>
      <div class="flex flex-wrap gap-2">
        <Button variant="primary" type="submit" disabled={busy()}>
          {busy() ? "Saving…" : "Save connection"}
        </Button>
        <Show when={connected()}>
          <Button disabled={busy()} onClick={runTest}>
            Test connection
          </Button>
          <Button
            variant="destructive"
            disabled={busy()}
            onClick={() => disconnectConnection(props.integration.id, name())}
          >
            Disconnect
          </Button>
        </Show>
        <Button onClick={() => props.edit.close()}>Cancel</Button>
      </div>
    </form>
  );
}
