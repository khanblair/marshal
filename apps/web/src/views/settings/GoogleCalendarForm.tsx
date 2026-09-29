import { Button, Field, Input } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import type { Integration } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";
import {
  authorizeGoogleCalendar,
  connectGoogleCalendar,
  disconnectConnection,
  testConnection,
} from "./integration-actions";

/**
 * Google Calendar's OAuth client (B8.3): the client id and secret from the Google Cloud console,
 * saved first. Access itself is granted through a separate step - the owner's own browser opens
 * Google's consent page - because Marshal cannot do that part on their behalf.
 */
export function GoogleCalendarForm(props: { integration: Integration; edit: EditState }) {
  const error = () => props.edit.errorFor(props.integration.id);
  const connected = () => props.integration.st === "connected";
  const clientSaved = () => props.integration.st !== "none" || connected();
  const [busy, setBusy] = createSignal(false);
  const name = () => props.integration.name;

  const submit = (form: HTMLFormElement): void => {
    const clientId = fieldValue(form, "clientId").trim();
    const clientSecret = fieldValue(form, "clientSecret").trim();
    if (!clientId || !clientSecret) {
      props.edit.fail("Marshal needs both a Google OAuth client id and secret.");
      return;
    }
    setBusy(true);
    void connectGoogleCalendar({ clientId, clientSecret }).finally(() => setBusy(false));
  };

  const openConsent = (): void => {
    setBusy(true);
    void authorizeGoogleCalendar()
      .then((answer) => {
        if (answer) window.open(answer.url, "_blank", "noopener,noreferrer");
      })
      .finally(() => setBusy(false));
  };

  const runTest = (): void => {
    setBusy(true);
    void testConnection(props.integration.id, name()).finally(() => setBusy(false));
  };

  return (
    <div class="flex flex-col gap-3">
      <form
        class="flex flex-col gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          submit(event.currentTarget);
        }}
      >
        <Field label="OAuth client id" hint="From the Google Cloud console's Credentials page.">
          <Input mono name="clientId" autocomplete="off" invalid={!!error()} />
        </Field>
        <Field label="OAuth client secret">
          <Input type="password" name="clientSecret" autocomplete="off" invalid={!!error()} />
        </Field>
        <Show when={error()}>
          <span class="text-small leading-4.5 text-status-danger-text">{error()}</span>
        </Show>
        <div class="flex flex-wrap gap-2">
          <Button variant="primary" type="submit" disabled={busy()}>
            {busy() ? "Saving…" : "Save client"}
          </Button>
          <Button onClick={() => props.edit.close()}>Cancel</Button>
        </div>
      </form>
      <Show when={clientSaved()}>
        <div class="flex flex-wrap gap-2 border-t border-border pt-3">
          <Button variant="primary" disabled={busy()} onClick={openConsent}>
            {connected() ? "Reconnect Google Calendar" : "Grant access"}
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
        </div>
        <span class="text-small leading-4.5 text-secondary">
          Opens Google's own sign-in page in a new tab. Once you grant access, come back here and
          press "Test connection".
        </span>
      </Show>
    </div>
  );
}
