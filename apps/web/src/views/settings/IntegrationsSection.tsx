import {
  Button,
  cx,
  Icon,
  IconLabel,
  type IconNameInput,
  SettingsPanel,
  SettingsSection,
} from "@marshal/ui";
import { batch, For, Match, Show, Switch } from "solid-js";
import { type Integration, M } from "~/mock";
import {
  DISCORD_ID,
  GCAL_ID,
  GMAIL_ID,
  OBSIDIAN_ID,
  TELEGRAM_ID,
  TRELLO_ID,
} from "~/sync/integrations";
import { GRID_MIN_240 } from "./auto-fit-grid";
import { DiscordForm } from "./DiscordForm";
import { createEditState, type EditState } from "./edit-state";
import { GitHubAppForm } from "./GitHubAppForm";
import { GmailForm } from "./GmailForm";
import { GoogleCalendarForm } from "./GoogleCalendarForm";
import { ObsidianPanel } from "./ObsidianPanel";
import { TelegramForm } from "./TelegramForm";
import { TestChecks } from "./TestChecks";
import { TrelloForm } from "./TrelloForm";

interface StatusSpec {
  label: string;
  icon: IconNameInput;
  textClass: string;
  button: string;
}

const STATUSES: Record<Integration["st"], StatusSpec> = {
  connected: {
    label: "Connected",
    icon: "check",
    textClass: "text-status-working-text",
    button: "Manage",
  },
  none: {
    label: "Not connected",
    icon: "circle-dashed",
    textClass: "text-secondary",
    button: "Connect",
  },
  error: {
    label: "Needs attention",
    icon: "x",
    textClass: "text-status-danger-text",
    button: "Reconnect",
  },
};

/** The design words every connected detail like a Gmail one, except Discord's. */
const CONNECTED_DETAIL = 'Labeled emails with "marshal" become cards';
const DISCORD_DETAIL = "Approvals and notices go to #marshal in your server";

/**
 * The mock's own connect: it flips the row to connected and says so. Only a row whose section is
 * still the mock's uses it; a daemon-backed row opens its form instead.
 */
function runIntegration(integration: Integration): void {
  if (integration.st === "connected") {
    M.toast(`${integration.name} settings opened`);
    return;
  }
  batch(() => {
    integration.st = "connected";
    integration.detail = integration.id === "discord" ? DISCORD_DETAIL : CONNECTED_DETAIL;
    M.toast(`${integration.name} connected`);
  });
}

function IntegrationCard(props: { integration: Integration; edit: EditState }) {
  const status = () => STATUSES[props.integration.st];
  const connected = () => props.integration.st === "connected";
  const onDaemon = () => M.connectionOnDaemon(props.integration.id);
  const open = () => props.edit.id() === props.integration.id;
  // A daemon-backed row's one button manages the connection: it opens the form, which carries the
  // save, the test, and the disconnect. A mock row keeps the design's own flip.
  const click = () =>
    onDaemon() ? props.edit.toggle(props.integration.id) : runIntegration(props.integration);
  return (
    <SettingsPanel class="flex flex-col gap-2.5 py-3.5 px-4">
      <div class="flex items-center gap-2.5">
        <Icon name={props.integration.icon} size={20} />
        <span class="flex-1 font-semibold">{props.integration.name}</span>
        <IconLabel
          icon={status().icon}
          size={12}
          class={cx("text-caption font-semibold", status().textClass)}
        >
          {status().label}
        </IconLabel>
      </div>
      {/* A connection nothing is stored for has no sentence, so the line is not drawn empty. */}
      <Show when={props.integration.detail}>
        <span
          class={cx(
            "flex-1 text-small leading-4.5",
            props.integration.st === "error" ? "text-status-danger-text" : "text-secondary",
          )}
        >
          {props.integration.detail}
        </span>
      </Show>
      <Button
        size={28}
        variant={connected() ? "secondary" : "primary"}
        class={cx("self-start", connected() && "font-semibold!")}
        onClick={click}
      >
        {status().button}
      </Button>
      {/* What the last connection test looked at, as a provider row shows its own. */}
      <Show when={props.integration.lastTest}>{(test) => <TestChecks test={test()} />}</Show>
      <Show when={onDaemon() && open()}>
        <Switch fallback={<GitHubAppForm integration={props.integration} edit={props.edit} />}>
          <Match when={props.integration.id === OBSIDIAN_ID}>
            <ObsidianPanel integration={props.integration} />
          </Match>
          <Match when={props.integration.id === TRELLO_ID}>
            <TrelloForm integration={props.integration} edit={props.edit} />
          </Match>
          <Match when={props.integration.id === GCAL_ID}>
            <GoogleCalendarForm integration={props.integration} edit={props.edit} />
          </Match>
          <Match when={props.integration.id === GMAIL_ID}>
            <GmailForm integration={props.integration} edit={props.edit} />
          </Match>
          <Match when={props.integration.id === TELEGRAM_ID}>
            <TelegramForm integration={props.integration} edit={props.edit} />
          </Match>
          <Match when={props.integration.id === DISCORD_ID}>
            <DiscordForm integration={props.integration} edit={props.edit} />
          </Match>
        </Switch>
      </Show>
    </SettingsPanel>
  );
}

/** Integrations: a card per service with its state and a Connect, Manage, or Reconnect button. */
export function IntegrationsSection() {
  const edit = createEditState();
  return (
    <SettingsSection title="Integrations">
      <div class={GRID_MIN_240}>
        <For each={M.S.integrations}>
          {(integration) => <IntegrationCard integration={integration} edit={edit} />}
        </For>
      </div>
    </SettingsSection>
  );
}
