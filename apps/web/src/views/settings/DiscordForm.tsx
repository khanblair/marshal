import { Button, Field, Input } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import type { Integration } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";
import { connectDiscord, disconnectConnection, testConnection } from "./integration-actions";

/**
 * Discord's whole setup (B9.3, section S29g): the bot's token and the channel notices go to. It is
 * the Telegram form's twin on purpose - the two connections are the same shape - and the daemon
 * writes the token to the keychain, never reads it back, and tests the bot as part of the same call.
 *
 * The channel id is the long number from the channel's own URL, because that is what Discord's API
 * takes; a channel name would have to be looked up per server, which the daemon does not do.
 */
export function DiscordForm(props: { integration: Integration; edit: EditState }) {
  const error = () => props.edit.errorFor(props.integration.id);
  const connected = () => props.integration.st !== "none";
  const [busy, setBusy] = createSignal(false);
  const name = () => props.integration.name;

  const submit = (form: HTMLFormElement): void => {
    const token = fieldValue(form, "token").trim();
    const channelId = fieldValue(form, "channelId").trim();
    if (!token) {
      props.edit.fail("Marshal needs the Discord bot's token to reach it.");
      return;
    }
    if (!channelId) {
      props.edit.fail("Enter the channel Marshal should send notices to.");
      return;
    }
    setBusy(true);
    void connectDiscord({ token, channelId })
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
        label="Bot token"
        hint="From the Discord developer portal. Marshal keeps it in the OS keychain."
      >
        <Input type="password" name="token" autocomplete="off" invalid={!!error()} />
      </Field>
      <Field label="Channel id" hint="The channel's own id, from its URL in the Discord app.">
        <Input mono name="channelId" autocomplete="off" invalid={!!error()} />
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
