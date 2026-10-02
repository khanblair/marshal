import { Button, Field, Input, Select } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import { type Integration, M } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";
import { openGoogleConsent } from "./google-consent";
import { connectGmail, disconnectConnection, testConnection } from "./integration-actions";

/**
 * Gmail's connection (B8.3): which label to watch, and which Marshal project a labeled email
 * becomes a card in. There is no client to save here - it uses the OAuth client saved on the Google
 * Calendar row - but Gmail asks Google for its own access, so Calendar never needs Gmail's scope.
 */
export function GmailForm(props: { integration: Integration; edit: EditState }) {
  const error = () => props.edit.errorFor(props.integration.id);
  const connected = () => props.integration.st === "connected";
  const [busy, setBusy] = createSignal(false);
  const name = () => props.integration.name;

  const submit = (form: HTMLFormElement): void => {
    const label = fieldValue(form, "label").trim();
    const projectId = fieldValue(form, "projectId");
    if (!label) {
      props.edit.fail("Choose the Gmail label Marshal should watch.");
      return;
    }
    if (!projectId) {
      props.edit.fail("Choose the Marshal project a labeled email becomes a card in.");
      return;
    }
    setBusy(true);
    void connectGmail({ label, projectId })
      .then((saved) => {
        if (saved) props.edit.close();
      })
      .finally(() => setBusy(false));
  };

  const openConsent = (): void => {
    setBusy(true);
    void openGoogleConsent(() => M.authorizeGmail())
      .then((opened) => {
        if (!opened)
          props.edit.fail("Marshal has no Google sign-in yet. Connect Google Calendar first.");
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
      <span class="text-small leading-4.5 text-secondary">
        Gmail asks Google for its own access, separate from Calendar's. It signs in the same way
        Google Calendar does: Marshal's own Google client, or the one you saved on that row.
      </span>
      <Field label="Label" hint='The Gmail label to watch, for example "marshal".'>
        <Input name="label" autocomplete="off" invalid={!!error()} />
      </Field>
      <Field label="Project" hint="The Marshal project a labeled email becomes a card in.">
        <Select
          name="projectId"
          options={M.S.projects.map((project) => ({ value: project.id, label: project.name }))}
        />
      </Field>
      <Show when={error()}>
        <span class="text-small leading-4.5 text-status-danger-text">{error()}</span>
      </Show>
      <div class="flex flex-wrap gap-2">
        <Button variant="primary" type="submit" disabled={busy()}>
          {busy() ? "Saving…" : "Save connection"}
        </Button>
        <Button disabled={busy()} onClick={openConsent}>
          {connected() ? "Reconnect Gmail" : "Grant access"}
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
