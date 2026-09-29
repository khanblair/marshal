import { Button, Field, Input, Select } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import { type Integration, M } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";
import { connectTrello, disconnectConnection, testConnection } from "./integration-actions";

/**
 * Trello's whole setup (B8.2): the API key, the token, the board, the list a new Trello card is
 * imported from, and the webhook secret with the callback URL Trello signed it for. The daemon
 * refuses a secret with no callback URL, or the other way round - they arrive together or not at
 * all - so both fields share one hint about that.
 */
export function TrelloForm(props: { integration: Integration; edit: EditState }) {
  const error = () => props.edit.errorFor(props.integration.id);
  const connected = () => props.integration.st !== "none";
  const [busy, setBusy] = createSignal(false);
  const name = () => props.integration.name;

  const submit = (form: HTMLFormElement): void => {
    const apiKey = fieldValue(form, "apiKey").trim();
    const token = fieldValue(form, "token").trim();
    const projectId = fieldValue(form, "projectId");
    const boardId = fieldValue(form, "boardId").trim();
    const newCardListId = fieldValue(form, "newCardListId").trim();
    const webhookSecret = fieldValue(form, "webhookSecret").trim();
    const callbackUrl = fieldValue(form, "callbackUrl").trim();
    if (!apiKey || !token) {
      props.edit.fail("Marshal needs both a Trello API key and a token.");
      return;
    }
    if (!projectId) {
      props.edit.fail("Choose the Marshal project this Trello board is linked to.");
      return;
    }
    if (!boardId) {
      props.edit.fail("Enter the id of the Trello board Marshal should watch.");
      return;
    }
    if (!!webhookSecret !== !!callbackUrl) {
      props.edit.fail("Enter both the webhook secret and the callback URL, or leave both empty.");
      return;
    }
    setBusy(true);
    void connectTrello({
      apiKey,
      token,
      projectId,
      boardId,
      newCardListId,
      webhookSecret,
      callbackUrl,
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
      <Field label="API key" hint="From your Trello Power-Up, or trello.com/app-key.">
        <Input mono name="apiKey" autocomplete="off" invalid={!!error()} />
      </Field>
      <Field label="Token" hint="Generated from the same page, using your API key.">
        <Input type="password" name="token" autocomplete="off" invalid={!!error()} />
      </Field>
      <Field label="Project" hint="The Marshal project this Trello board is linked to.">
        <Select
          name="projectId"
          options={M.S.projects.map((project) => ({ value: project.id, label: project.name }))}
        />
      </Field>
      <Field label="Board id" hint="The id in the board's own URL on trello.com.">
        <Input mono name="boardId" autocomplete="off" invalid={!!error()} />
      </Field>
      <Field label="Import list id" hint="A card added here becomes a Marshal card. Optional.">
        <Input mono name="newCardListId" autocomplete="off" />
      </Field>
      <Field label="Webhook secret" hint="Set when you register the webhook. Optional.">
        <Input type="password" name="webhookSecret" autocomplete="off" invalid={!!error()} />
      </Field>
      <Field label="Callback URL" hint="The address you registered the webhook with. Optional.">
        <Input mono name="callbackUrl" autocomplete="off" invalid={!!error()} />
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
