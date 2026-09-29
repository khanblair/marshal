import { Button, Field, Input, Select } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import { type Integration, M } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";
import { connectGmail, disconnectConnection, testConnection } from "./integration-actions";

/**
 * Gmail's connection (B8.3): which label to watch, and which Marshal project a labeled email
 * becomes a card in. There is no client to save here - Google Calendar's own OAuth client and
 * consent, once granted (the row above this one), cover Gmail too.
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
        Uses Google Calendar's own sign-in above. Connect that first if it is not connected yet.
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
