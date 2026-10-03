import { Button, Field, type SegmentOption, Select } from "@marshal/ui";
import { createSignal, Match, Show, Switch } from "solid-js";
import { type Integration, M } from "~/mock";
import { ConnectionStored } from "./ConnectionStored";
import { ErrorLine, SaveRow, ValueField } from "./connect-fields";
import {
  createGmailConnect,
  type GmailConnectController,
  type GmailConnectProps,
} from "./gmail-connect";
import { WaysDialog } from "./WaysDialog";

type Way = "label" | "access";

const WAYS: readonly SegmentOption<Way>[] = [
  { value: "label", label: "What to watch" },
  { value: "access", label: "Google access" },
];

/** The first tab: which label to watch, and which Marshal project a labeled email becomes a card in. */
function LabelTab(props: { integration: Integration; gmail: GmailConnectController }) {
  const gmail = () => props.gmail;
  return (
    <form
      class="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        gmail().save();
      }}
    >
      <Show when={gmail().connected()}>
        <ConnectionStored integration={props.integration} onDisconnected={() => undefined} />
      </Show>
      <ValueField
        label="Label"
        hint='The Gmail label to watch, for example "marshal".'
        name="label"
        mono={false}
        value={gmail().draft.label}
        onValue={(value) => gmail().set("label", value)}
        invalid={!!gmail().error()}
      />
      <Field label="Project" hint="The Marshal project a labeled email becomes a card in.">
        <Select
          name="projectId"
          value={gmail().draft.projectId}
          options={M.S.projects.map((project) => ({ value: project.id, label: project.name }))}
          onChange={(event) => gmail().set("projectId", event.currentTarget.value)}
        />
      </Field>
      <ErrorLine message={gmail().error()} />
      <SaveRow busy={gmail().busy()} />
    </form>
  );
}

/** The second tab: Gmail's own access from Google, separate from Calendar's. */
function AccessTab(props: { integration: Integration; gmail: GmailConnectController }) {
  const gmail = () => props.gmail;
  return (
    <div class="flex flex-col gap-3">
      <span class="text-small leading-4.5 text-secondary">
        Gmail asks Google for its own access, separate from Calendar's. It signs in the same way
        Google Calendar does: Marshal's own Google client, or the one you saved there. Marshal can
        only read, and only the label you chose.
      </span>
      <Show when={gmail().connected()}>
        <ConnectionStored integration={props.integration} onDisconnected={() => undefined} />
      </Show>
      <ErrorLine message={gmail().error()} />
      <div class="flex flex-wrap gap-2">
        <Button variant="primary" disabled={gmail().busy()} onClick={gmail().grant}>
          {gmail().connected() ? "Reconnect Gmail" : "Grant access"}
        </Button>
      </div>
    </div>
  );
}

/**
 * Connecting Gmail (B8.3), as a dialog over Settings with two tabs: the label and project, and
 * Gmail's own Google access. It is connected once it has both, in either order, and saving the label
 * before access moves on to the access tab.
 */
export function GmailConnectDialog(props: GmailConnectProps) {
  const [way, setWay] = createSignal<Way>("label");
  const gmail = createGmailConnect(props, () => setWay("access"));
  return (
    <WaysDialog
      titleId="gmail-title"
      title="Connect Gmail"
      label="Gmail settings"
      ways={WAYS}
      value={way()}
      onValueChange={setWay}
      onClose={() => props.edit.close()}
    >
      <Switch>
        <Match when={way() === "label"}>
          <LabelTab integration={props.integration} gmail={gmail} />
        </Match>
        <Match when={way() === "access"}>
          <AccessTab integration={props.integration} gmail={gmail} />
        </Match>
      </Switch>
    </WaysDialog>
  );
}
