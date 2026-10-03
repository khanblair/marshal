import { Field, type SegmentOption, Select } from "@marshal/ui";
import { createSignal, Match, Show, Switch } from "solid-js";
import { type Integration, M } from "~/mock";
import { ConnectionStored } from "./ConnectionStored";
import { ErrorLine, SaveRow, ValueField } from "./connect-fields";
import type { EditState } from "./edit-state";
import { createTrelloConnect, type TrelloConnectController } from "./trello-connect";
import { WaysDialog } from "./WaysDialog";

type Way = "connection" | "sync";

const WAYS: readonly SegmentOption<Way>[] = [
  { value: "connection", label: "Connection" },
  { value: "sync", label: "Import and webhook" },
];

/** The link that makes a Trello token for the key typed, with the read and write access Marshal needs. */
const tokenUrl = (apiKey: string): string =>
  `https://trello.com/1/authorize?expiration=never&name=Marshal&scope=read,write&response_type=token&key=${encodeURIComponent(apiKey.trim())}`;

/** The first tab: the key and token that stand for the person, and which board is linked to which project. */
function ConnectionTab(props: { integration: Integration; trello: TrelloConnectController }) {
  const trello = () => props.trello;
  return (
    <form
      class="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        trello().save();
      }}
    >
      <Show when={trello().connected()}>
        <ConnectionStored integration={props.integration} onDisconnected={() => undefined} />
      </Show>
      <ValueField
        label="API key"
        hint="From your Trello Power-Up, or trello.com/app-key."
        name="apiKey"
        value={trello().draft.apiKey}
        onValue={(value) => trello().set("apiKey", value)}
        invalid={!!trello().error()}
      />
      <ValueField
        label="Token"
        hint="Generated from the same page, using your API key."
        name="token"
        secret
        value={trello().draft.token}
        onValue={(value) => trello().set("token", value)}
        invalid={!!trello().error()}
      />
      <Show when={trello().draft.apiKey.trim()}>
        <a
          href={tokenUrl(trello().draft.apiKey)}
          target="_blank"
          rel="noopener noreferrer"
          class="self-start text-small underline"
        >
          Open Trello to make a token for this key
        </a>
      </Show>
      <Field label="Project" hint="The Marshal project this Trello board is linked to.">
        <Select
          name="projectId"
          value={trello().draft.projectId}
          options={M.S.projects.map((project) => ({ value: project.id, label: project.name }))}
          onChange={(event) => trello().set("projectId", event.currentTarget.value)}
        />
      </Field>
      <ValueField
        label="Board id"
        hint="The id in the board's own URL on trello.com."
        name="boardId"
        value={trello().draft.boardId}
        onValue={(value) => trello().set("boardId", value)}
        invalid={!!trello().error()}
      />
      <ErrorLine message={trello().error()} />
      <SaveRow busy={trello().busy()} />
    </form>
  );
}

/** The second tab: which list imports new cards, and the webhook that lets Trello tell Marshal at once. */
function SyncTab(props: { integration: Integration; trello: TrelloConnectController }) {
  const trello = () => props.trello;
  return (
    <form
      class="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        trello().save();
      }}
    >
      <Show when={trello().connected()}>
        <ConnectionStored integration={props.integration} onDisconnected={() => undefined} />
      </Show>
      <ValueField
        label="Import list id"
        hint="A card added here becomes a Marshal card. Optional."
        name="newCardListId"
        value={trello().draft.newCardListId}
        onValue={(value) => trello().set("newCardListId", value)}
      />
      <ValueField
        label="Webhook secret"
        hint="Set when you register the webhook. Optional, but give the callback URL with it."
        name="webhookSecret"
        secret
        value={trello().draft.webhookSecret}
        onValue={(value) => trello().set("webhookSecret", value)}
        invalid={!!trello().error()}
      />
      <ValueField
        label="Callback URL"
        hint="The address you registered the webhook with. Optional, but give the secret with it."
        name="callbackUrl"
        value={trello().draft.callbackUrl}
        onValue={(value) => trello().set("callbackUrl", value)}
        invalid={!!trello().error()}
      />
      <ErrorLine message={trello().error()} />
      <SaveRow busy={trello().busy()} />
    </form>
  );
}

/**
 * Connecting Trello (B8.2), as a dialog over Settings with two tabs: the connection itself, and the
 * optional import list and webhook. Both tabs fill in the one connection and either can save it.
 */
export function TrelloConnectDialog(props: { integration: Integration; edit: EditState }) {
  const [way, setWay] = createSignal<Way>("connection");
  const trello = createTrelloConnect(props.integration, props.edit);
  return (
    <WaysDialog
      titleId="trello-title"
      title="Connect Trello"
      label="Trello settings"
      ways={WAYS}
      value={way()}
      onValueChange={setWay}
      onClose={() => props.edit.close()}
    >
      <Switch>
        <Match when={way() === "connection"}>
          <ConnectionTab integration={props.integration} trello={trello} />
        </Match>
        <Match when={way() === "sync"}>
          <SyncTab integration={props.integration} trello={trello} />
        </Match>
      </Switch>
    </WaysDialog>
  );
}
