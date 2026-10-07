import type { SegmentOption } from "@marshal/ui";
import { createSignal, type JSX, Match, Show, Switch } from "solid-js";
import type { Integration } from "~/mock";
import { ConnectionStored } from "./ConnectionStored";
import { ErrorLine, SaveRow, Steps, ValueField } from "./connect-fields";
import { createDiscordConnect, type DiscordConnectController } from "./discord-connect";
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
import { WaysDialog } from "./WaysDialog";

type Way = "bot" | "channel";

const WAYS: readonly SegmentOption<Way>[] = [
  { value: "bot", label: "The bot" },
  { value: "channel", label: "The channel" },
];

function Tab(props: {
  integration: Integration;
  discord: DiscordConnectController;
  steps: readonly string[];
  children: JSX.Element;
}) {
  const discord = () => props.discord;
  return (
    <form
      class="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        discord().save();
      }}
    >
      <Show when={discord().connected()}>
        <ConnectionStored integration={props.integration} onDisconnected={() => undefined} />
      </Show>
      <Steps steps={props.steps} />
      {props.children}
      <ErrorLine message={discord().error()} />
      <SaveRow busy={discord().busy()} />
    </form>
  );
}

/**
 * Connecting Discord (B9.3, section S29g), as a dialog over Settings with two tabs: the bot, and the
 * channel its notices go to. Both tabs fill in the one connection and either can save it. The token
 * is kept in the OS keychain.
 */
export function DiscordConnectDialog(props: { integration: Integration; edit: EditState }) {
  const [way, setWay] = createSignal<Way>("bot");
  const discord = createDiscordConnect(props.integration, props.edit, setWay);
  return (
    <WaysDialog
      titleId="discord-title"
      title="Connect Discord"
      label="Discord settings"
      ways={WAYS}
      value={way()}
      onValueChange={setWay}
      onClose={() => props.edit.close()}
    >
      <Switch>
        <Match when={way() === "bot"}>
          <Tab integration={props.integration} discord={discord} steps={DISCORD_BOT_STEPS}>
            <a
              href={DISCORD_PORTAL_URL}
              target="_blank"
              rel="noopener noreferrer"
              class="self-start text-small underline"
            >
              Open the Discord developer portal
            </a>
            <ValueField
              label="Bot token"
              hint={discord.connected() ? DISCORD_TOKEN_SAVED_HINT : DISCORD_TOKEN_HINT}
              placeholder={discord.connected() ? DISCORD_TOKEN_DOTS : undefined}
              name="token"
              secret
              value={discord.draft.token}
              onValue={(value) => discord.set("token", value)}
              invalid={discord.invalid("token")}
            />
          </Tab>
        </Match>
        <Match when={way() === "channel"}>
          <Tab integration={props.integration} discord={discord} steps={DISCORD_CHANNEL_STEPS}>
            <ValueField
              label="Channel id"
              hint={DISCORD_CHANNEL_HINT}
              name="channelId"
              value={discord.draft.channelId}
              onValue={(value) => discord.set("channelId", value)}
              invalid={discord.invalid("channelId")}
            />
          </Tab>
        </Match>
      </Switch>
    </WaysDialog>
  );
}
