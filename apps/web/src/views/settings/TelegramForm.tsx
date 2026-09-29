import { Button, Field, Input } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import type { Integration } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";
import { connectTelegram, disconnectConnection, testConnection } from "./integration-actions";

/**
 * Telegram's whole setup (B9.3, section S29f): the bot's token and the chat notices go to. The
 * daemon writes the token to the keychain, never reads it back, and tests the bot as part of the
 * same call, so saving and testing are one press.
 *
 * The chat is a numeric id (the person's own chat with the bot) or a channel name such as
 * "@my_channel"; the field accepts either because the daemon sends a number as a number and a name
 * as a name.
 */
export function TelegramForm(props: { integration: Integration; edit: EditState }) {
  const error = () => props.edit.errorFor(props.integration.id);
  const connected = () => props.integration.st !== "none";
  const [busy, setBusy] = createSignal(false);
  const name = () => props.integration.name;

  const submit = (form: HTMLFormElement): void => {
    const token = fieldValue(form, "token").trim();
    const chatId = fieldValue(form, "chatId").trim();
    if (!token) {
      props.edit.fail("Marshal needs the Telegram bot's token to reach it.");
      return;
    }
    if (!chatId) {
      props.edit.fail("Enter the chat Marshal should send notices to.");
      return;
    }
    setBusy(true);
    void connectTelegram({ token, chatId })
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
        hint="From @BotFather on Telegram. Marshal keeps it in the OS keychain."
      >
        <Input type="password" name="token" autocomplete="off" invalid={!!error()} />
      </Field>
      <Field label="Chat" hint="Your chat's numeric id, or a channel name such as @my_channel.">
        <Input mono name="chatId" autocomplete="off" invalid={!!error()} />
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
