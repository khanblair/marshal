import { Button, type SegmentOption } from "@marshal/ui";
import { createSignal, type JSX, Match, Show, Switch } from "solid-js";
import type { Integration } from "~/mock";
import { ConnectionStored } from "./ConnectionStored";
import { ErrorLine, SaveRow, Steps, ValueField } from "./connect-fields";
import type { EditState } from "./edit-state";
import {
  createNtfyConnect,
  type NtfyConnectController,
  type NtfyWay,
  randomTopic,
} from "./ntfy-connect";
import { WaysDialog } from "./WaysDialog";

const WAYS: readonly SegmentOption<NtfyWay>[] = [
  { value: "public", label: "ntfy.sh" },
  { value: "own", label: "My own server" },
];

const PUBLIC_STEPS = [
  "Install the ntfy app on your phone.",
  "Make a topic below, and subscribe to the same topic in the app.",
  "Save. Alerts then arrive as ordinary notifications, even while Marshal is closed.",
];

const TOPIC_HINT =
  "Pick a long, hard-to-guess name. On a public server anyone who knows it can read your alerts.";

function Tab(props: {
  integration: Integration;
  ntfy: NtfyConnectController;
  way: NtfyWay;
  steps?: readonly string[];
  children: JSX.Element;
}) {
  const ntfy = () => props.ntfy;
  return (
    <form
      class="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        ntfy().save(props.way);
      }}
    >
      <Show when={ntfy().connected()}>
        <ConnectionStored integration={props.integration} onDisconnected={() => undefined} />
      </Show>
      <Show when={props.steps}>{(steps) => <Steps steps={steps()} />}</Show>
      {props.children}
      <ErrorLine message={ntfy().error()} />
      <SaveRow busy={ntfy().busy()} />
    </form>
  );
}

/**
 * Connecting ntfy (B9.3, section S29h), as a dialog over Settings with two tabs: ntfy's own public
 * server, which needs only a topic, and a server of the person's own, with its address and an
 * optional access token. Both tabs share the topic. The daemon publishes a test message on saving.
 */
export function NtfyConnectDialog(props: { integration: Integration; edit: EditState }) {
  const [way, setWay] = createSignal<NtfyWay>("public");
  const ntfy = createNtfyConnect(props.integration, props.edit);
  return (
    <WaysDialog
      titleId="ntfy-title"
      title="Connect ntfy"
      label="How to connect ntfy"
      ways={WAYS}
      value={way()}
      onValueChange={setWay}
      onClose={() => props.edit.close()}
    >
      <Switch>
        <Match when={way() === "public"}>
          <Tab integration={props.integration} ntfy={ntfy} way="public" steps={PUBLIC_STEPS}>
            <ValueField
              label="Topic"
              hint={TOPIC_HINT}
              name="topic"
              value={ntfy.draft.topic}
              onValue={(value) => ntfy.set("topic", value)}
              invalid={!!ntfy.error()}
            />
            <Button class="self-start" onClick={() => ntfy.set("topic", randomTopic())}>
              Make me a topic
            </Button>
          </Tab>
        </Match>
        <Match when={way() === "own"}>
          <Tab integration={props.integration} ntfy={ntfy} way="own">
            <ValueField
              label="Server"
              hint="The address of your own ntfy server."
              name="server"
              placeholder="https://ntfy.example.com"
              value={ntfy.draft.server}
              onValue={(value) => ntfy.set("server", value)}
              invalid={!!ntfy.error()}
            />
            <ValueField
              label="Topic"
              hint={TOPIC_HINT}
              name="topic"
              value={ntfy.draft.topic}
              onValue={(value) => ntfy.set("topic", value)}
              invalid={!!ntfy.error()}
            />
            <ValueField
              label="Access token"
              hint="Only for a server that needs one. Marshal keeps it in the OS keychain."
              name="token"
              secret
              value={ntfy.draft.token}
              onValue={(value) => ntfy.set("token", value)}
            />
          </Tab>
        </Match>
      </Switch>
    </WaysDialog>
  );
}
