import type { SegmentOption } from "@marshal/ui";
import { createSignal, type JSX, Match, Show, Switch } from "solid-js";
import type { Integration } from "~/mock";
import { ConnectionStored } from "./ConnectionStored";
import { ErrorLine, SaveRow, Steps, ValueField } from "./connect-fields";
import { createDiscordConnect, type DiscordConnectController } from "./discord-connect";
import type { EditState } from "./edit-state";
import { WaysDialog } from "./WaysDialog";

type Way = "bot" | "channel";

const WAYS: readonly SegmentOption<Way>[] = [
  { value: "bot", label: "The bot" },
  { value: "channel", label: "The channel" },
];

const BOT_STEPS = [
  "In the Discord developer portal, make a New Application, open its Bot page, and copy the token.",
  "To answer by typing, and not only with the buttons, switch on Message Content Intent there too.",
  "Under OAuth2, make a link with the bot scope and the Send Messages permission, and open it to add the bot to your server.",
];

const CHANNEL_STEPS = [
  "In Discord, open User Settings, then Advanced, and switch on Developer Mode.",
  "Right-click the channel notices should go to, and choose Copy Channel ID.",
  "Paste it below.",
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
  const discord = createDiscordConnect(props.integration, props.edit);
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
          <Tab integration={props.integration} discord={discord} steps={BOT_STEPS}>
            <a
              href="https://discord.com/developers/applications"
              target="_blank"
              rel="noopener noreferrer"
              class="self-start text-small underline"
            >
              Open the Discord developer portal
            </a>
            <ValueField
              label="Bot token"
              hint="Marshal keeps it in the OS keychain."
              name="token"
              secret
              value={discord.draft.token}
              onValue={(value) => discord.set("token", value)}
              invalid={!!discord.error()}
            />
          </Tab>
        </Match>
        <Match when={way() === "channel"}>
          <Tab integration={props.integration} discord={discord} steps={CHANNEL_STEPS}>
            <ValueField
              label="Channel id"
              hint="The channel's own id, the long number Discord copies."
              name="channelId"
              value={discord.draft.channelId}
              onValue={(value) => discord.set("channelId", value)}
              invalid={!!discord.error()}
            />
          </Tab>
        </Match>
      </Switch>
    </WaysDialog>
  );
}
