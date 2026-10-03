import type { SegmentOption } from "@marshal/ui";
import { createSignal, Match, Switch } from "solid-js";
import type { Integration } from "~/mock";
import type { EditState } from "./edit-state";
import { TelegramFindChat } from "./TelegramFindChat";
import { TelegramManual } from "./TelegramManual";
import { createTelegramConnect } from "./telegram-connect";
import { WaysDialog } from "./WaysDialog";

type Way = "find" | "manual";

const WAYS: readonly SegmentOption<Way>[] = [
  { value: "find", label: "Find my chat" },
  { value: "manual", label: "Enter the chat" },
];

/**
 * Connecting Telegram (B9.3, section S29f), as a dialog over Settings: paste the bot's token and let
 * Marshal find the chat, or type the chat yourself. Both tabs share the token and the chat, so what
 * one finds, the other shows, and both read the one stored connection.
 */
export function TelegramConnectDialog(props: { integration: Integration; edit: EditState }) {
  const [way, setWay] = createSignal<Way>("find");
  const telegram = createTelegramConnect(props.integration, props.edit);
  return (
    <WaysDialog
      titleId="telegram-title"
      title="Connect Telegram"
      label="How to connect Telegram"
      ways={WAYS}
      value={way()}
      onValueChange={setWay}
      onClose={() => props.edit.close()}
    >
      <Switch>
        <Match when={way() === "find"}>
          <TelegramFindChat integration={props.integration} edit={props.edit} telegram={telegram} />
        </Match>
        <Match when={way() === "manual"}>
          <TelegramManual integration={props.integration} telegram={telegram} />
        </Match>
      </Switch>
    </WaysDialog>
  );
}
