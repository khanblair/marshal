import { Button, Field, Input } from "@marshal/ui";
import { Show } from "solid-js";
import type { Integration } from "~/mock";
import { ConnectionStored } from "./ConnectionStored";
import type { TelegramConnectController } from "./telegram-connect";

/**
 * The second tab: the bot's token and the chat typed by hand. The chat is a numeric id, or a channel
 * name such as "@my_channel"; the daemon sends a number as a number and a name as a name.
 */
export function TelegramManual(props: {
  integration: Integration;
  telegram: TelegramConnectController;
}) {
  const telegram = () => props.telegram;
  return (
    <form
      class="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        telegram().save();
      }}
    >
      <Show when={telegram().connected()}>
        <ConnectionStored integration={props.integration} onDisconnected={() => undefined} />
      </Show>
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
      <Field label="Chat" hint="Your chat's numeric id, or a channel name such as @my_channel.">
        <Input
          mono
          name="chatId"
          autocomplete="off"
          spellcheck={false}
          value={telegram().chat()}
          invalid={!!telegram().error()}
          onInput={(event) => telegram().setChat(event.currentTarget.value)}
        />
      </Field>
      <Show when={telegram().error()}>
        <span role="alert" class="text-small leading-4.5 text-status-danger-text">
          {telegram().error()}
        </span>
      </Show>
      <div class="flex flex-wrap gap-2">
        <Button variant="primary" type="submit" disabled={telegram().busy()}>
          {telegram().busy() ? "Saving…" : "Save connection"}
        </Button>
      </div>
    </form>
  );
}
