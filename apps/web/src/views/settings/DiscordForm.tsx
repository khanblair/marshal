import { Button, Field, Input } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import type { Integration } from "~/mock";
import { Steps } from "./connect-fields";
import {
  DISCORD_BOT_STEPS,
  DISCORD_CHANNEL_HINT,
  DISCORD_CHANNEL_STEPS,
  DISCORD_PORTAL_URL,
  DISCORD_TOKEN_DOTS,
  DISCORD_TOKEN_HINT,
  DISCORD_TOKEN_SAVED_HINT,
} from "./discord-words";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";
import { connectDiscord, disconnectConnection, testConnection } from "./integration-actions";

/**
 * Discord's whole setup (B9.3, section S29g): the bot's token and the channel notices go to. It is
 * the Telegram form's twin on purpose - the two connections are the same shape - and the daemon
 * writes the token to the keychain, never reads it back, and tests the bot as part of the same call.
 *
 * The channel id is the long number Discord copies for the channel, because that is what Discord's API
 * takes; a channel name would have to be looked up per server, which the daemon does not do. The
 * daemon also reads a pasted channel link, since a link carries the server's number first.
 */
export function DiscordForm(props: { integration: Integration; edit: EditState }) {
  const error = () => props.edit.errorFor(props.integration.id);
  const connected = () => props.integration.st !== "none";
  const [busy, setBusy] = createSignal(false);
  // The field a refusal is about, so only it is marked; a refusal from the daemon marks both.
  const [empty, setEmpty] = createSignal<"token" | "channelId" | null>(null);
  const invalid = (field: "token" | "channelId") => !!error() && (!empty() || empty() === field);
  const name = () => props.integration.name;

  const submit = (form: HTMLFormElement): void => {
    const token = fieldValue(form, "token").trim();
    const channelId = fieldValue(form, "channelId").trim();
    setEmpty(null);
    // A token left empty keeps the one already saved, so only a new connection needs one.
    if (!token && !connected()) {
      setEmpty("token");
      props.edit.fail("Marshal needs the Discord bot's token to reach it.");
      return;
    }
    if (!channelId) {
      setEmpty("channelId");
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
      <Steps steps={DISCORD_BOT_STEPS} />
      <a
        href={DISCORD_PORTAL_URL}
        target="_blank"
        rel="noopener noreferrer"
        class="self-start text-small underline"
      >
        Open the Discord developer portal
      </a>
      <Field label="Bot token" hint={connected() ? DISCORD_TOKEN_SAVED_HINT : DISCORD_TOKEN_HINT}>
        <Input
          type="password"
          name="token"
          autocomplete="off"
          placeholder={connected() ? DISCORD_TOKEN_DOTS : undefined}
          invalid={invalid("token")}
        />
      </Field>
      <Steps steps={DISCORD_CHANNEL_STEPS} />
      <Field label="Channel id" hint={DISCORD_CHANNEL_HINT}>
        <Input
          mono
          name="channelId"
          autocomplete="off"
          value={props.integration.target ?? ""}
          invalid={invalid("channelId")}
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
