import { Button, Field, Input } from "@marshal/ui";
import { Show } from "solid-js";
import type { Integration } from "~/mock";
import { ConnectionStored } from "./ConnectionStored";
import type { EditState } from "./edit-state";
import type { TelegramConnectController } from "./telegram-connect";

const STEPS = [
  "In Telegram, message @BotFather, send /newbot, and copy the token it gives you.",
  "Paste the token below.",
  "Open your new bot in Telegram, press Start, and send it any message.",
  "Press Find my chat here, then Save connection.",
];

/**
 * The first tab: make a bot, paste its token, and let Marshal find the chat that wrote to it, so no
 * chat id is typed. The token is kept in the OS keychain once saved.
 */
export function TelegramFindChat(props: {
  integration: Integration;
  edit: EditState;
  telegram: TelegramConnectController;
}) {
  const telegram = () => props.telegram;
  const save = (): void => {
    if (!telegram().chat().trim() && telegram().token().trim()) {
      props.edit.fail("Press Find my chat first, or type the chat on the other tab.");
      return;
    }
    telegram().save();
  };
  return (
    <form
      class="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        save();
      }}
    >
      <Show when={telegram().connected()}>
        <ConnectionStored integration={props.integration} onDisconnected={() => undefined} />
      </Show>
      <ol class="m-0 pl-5 flex flex-col gap-0.5 text-small leading-4.5 text-secondary">
        {STEPS.map((step) => (
          <li>{step}</li>
        ))}
      </ol>
      <Field
        label="Bot token"
        hint="From @BotFather on Telegram. Marshal keeps it in the OS keychain."
      >
        <Input
          mono
          type="password"
          name="token"
          autocomplete="off"
          spellcheck={false}
          value={telegram().token()}
          invalid={!!telegram().error()}
          onInput={(event) => telegram().setToken(event.currentTarget.value)}
        />
      </Field>
      <Show when={telegram().found()}>
        <span role="status" class="text-small leading-4.5 text-secondary">
          {telegram().found()}
        </span>
      </Show>
      <Show when={telegram().error()}>
        <span role="alert" class="text-small leading-4.5 text-status-danger-text">
          {telegram().error()}
        </span>
      </Show>
      <div class="flex flex-wrap gap-2">
        <Button disabled={telegram().busy()} onClick={telegram().find}>
          Find my chat
        </Button>
        <Button variant="primary" type="submit" disabled={telegram().busy()}>
          {telegram().busy() ? "Saving…" : "Save connection"}
        </Button>
      </div>
    </form>
  );
}
