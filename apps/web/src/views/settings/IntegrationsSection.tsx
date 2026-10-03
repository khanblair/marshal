import {
  Button,
  cx,
  Icon,
  IconButton,
  IconLabel,
  type IconNameInput,
  ItemText,
  SettingsPanel,
  SettingsSection,
} from "@marshal/ui";
import { batch, createSignal, For, Match, Show, Switch } from "solid-js";
import { type Integration, M } from "~/mock";
import {
  DISCORD_ID,
  GCAL_ID,
  GITHUB_ID,
  GMAIL_ID,
  NTFY_ID,
  OBSIDIAN_ID,
  TELEGRAM_ID,
  TRELLO_ID,
} from "~/sync/integrations";
import { GRID_MIN_240 } from "./auto-fit-grid";
import { DiscordConnectDialog } from "./DiscordConnectDialog";
import { createEditState, type EditState } from "./edit-state";
import { GitHubConnectDialog } from "./GitHubConnectDialog";
import { GmailConnectDialog } from "./GmailConnectDialog";
import { GoogleConnectDialog } from "./GoogleConnectDialog";
import { testConnection } from "./integration-actions";
import {
  type IntegrationsView,
  readExpandedIntegrations,
  readIntegrationsView,
  writeExpandedIntegrations,
  writeIntegrationsView,
} from "./integrations-view";
import { NtfyConnectDialog } from "./NtfyConnectDialog";
import { ObsidianPanel } from "./ObsidianPanel";
import { TelegramConnectDialog } from "./TelegramConnectDialog";
import { TestChecks } from "./TestChecks";
import { TrelloConnectDialog } from "./TrelloConnectDialog";

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

/** What one connection's card and its list row share: how it reads and what its one button does. */
function useIntegration(integration: Integration, edit: EditState, fold: Fold) {
  const status = () => STATUSES[integration.st];
  const onDaemon = () => M.connectionOnDaemon(integration.id);
  const showForm = () => onDaemon() && edit.id() === integration.id;
  return {
    status,
    connected: () => integration.st === "connected",
    showForm,
    // Only a connection with something stored has anything to test.
    testable: () => onDaemon() && integration.st !== "none",
    // A form that is open keeps its connection unfolded, so pressing a button never hides its answer.
    open: () => !fold.collapsed(integration.id) || showForm(),
    // A daemon-backed row's one button manages the connection: it opens the form (GitHub's is a
    // dialog), which carries the save, the test, and the disconnect. A mock row keeps the design's own flip.
    click: () => (onDaemon() ? edit.toggle(integration.id) : runIntegration(integration)),
  };
}

/** Which connections are unfolded, shared by both layouts and kept per browser. All start folded. */
interface Fold {
  collapsed: (id: string) => boolean;
  toggle: (id: string) => void;
  expand: (id: string) => void;
}

function createFold(): Fold {
  const [open, setOpen] = createSignal<readonly string[]>(readExpandedIntegrations());
  const save = (next: readonly string[]): void => {
    setOpen(next);
    writeExpandedIntegrations(next);
  };
  const expand = (id: string): void => {
    if (!open().includes(id)) save([...open(), id]);
  };
  return {
    collapsed: (id) => !open().includes(id),
    toggle: (id) =>
      open().includes(id) ? save(open().filter((entry) => entry !== id)) : expand(id),
    expand,
  };
}

/** The chevron at the start of a header that folds the connection to just that header. */
function FoldToggle(props: { name: string; open: boolean; bodyId: string; onToggle: () => void }) {
  return (
    <IconButton
      size={24}
      icon={props.open ? "chevron-down" : "chevron-right"}
      label={`${props.open ? "Collapse" : "Expand"} ${props.name}`}
      aria-expanded={props.open}
      aria-controls={props.bodyId}
      onClick={props.onToggle}
    />
  );
}

/** Runs the connection's own test and unfolds it, so the result is in view. */
function TestButton(props: { integration: Integration; fold: Fold }) {
  const [busy, setBusy] = createSignal(false);
  const run = (): void => {
    props.fold.expand(props.integration.id);
    setBusy(true);
    void testConnection(props.integration.id, props.integration.name).finally(() => setBusy(false));
  };
  return (
    <Button size={28} disabled={busy()} onClick={run}>
      {busy() ? "Testing…" : "Test"}
    </Button>
  );
}

/** The form a daemon-backed connection opens under its row or card. */
function IntegrationForm(props: { integration: Integration; edit: EditState }) {
  return (
    <Switch>
      <Match when={props.integration.id === GITHUB_ID}>
        <GitHubConnectDialog integration={props.integration} edit={props.edit} />
      </Match>
      <Match when={props.integration.id === OBSIDIAN_ID}>
        <ObsidianPanel integration={props.integration} />
      </Match>
      <Match when={props.integration.id === TRELLO_ID}>
        <TrelloConnectDialog integration={props.integration} edit={props.edit} />
      </Match>
      <Match when={props.integration.id === GCAL_ID}>
        <GoogleConnectDialog integration={props.integration} edit={props.edit} />
      </Match>
      <Match when={props.integration.id === GMAIL_ID}>
        <GmailConnectDialog integration={props.integration} edit={props.edit} />
      </Match>
      <Match when={props.integration.id === TELEGRAM_ID}>
        <TelegramConnectDialog integration={props.integration} edit={props.edit} />
      </Match>
      <Match when={props.integration.id === DISCORD_ID}>
        <DiscordConnectDialog integration={props.integration} edit={props.edit} />
      </Match>
      <Match when={props.integration.id === NTFY_ID}>
        <NtfyConnectDialog integration={props.integration} edit={props.edit} />
      </Match>
    </Switch>
  );
}

interface ItemProps {
  integration: Integration;
  edit: EditState;
  fold: Fold;
}

function IntegrationCard(props: ItemProps) {
  const row = useIntegration(props.integration, props.edit, props.fold);
  const bodyId = `integration-body-${props.integration.id}`;
  return (
    <SettingsPanel
      data-integration={props.integration.id}
      class="flex flex-col gap-2.5 py-3.5 px-4"
    >
      <div class="flex items-center gap-2.5">
        <FoldToggle
          name={props.integration.name}
          open={row.open()}
          bodyId={bodyId}
          onToggle={() => props.fold.toggle(props.integration.id)}
        />
        <Icon name={props.integration.icon} size={20} />
        <span class="flex-1 font-semibold">{props.integration.name}</span>
        <IconLabel
          icon={row.status().icon}
          size={12}
          class={cx("text-caption font-semibold", row.status().textClass)}
        >
          {row.status().label}
        </IconLabel>
      </div>
      {/* A connection nothing is stored for has no sentence, so the line is not drawn empty. */}
      <Show when={row.open() && props.integration.detail}>
        <span
          class={cx(
            "flex-1 text-small leading-4.5",
            props.integration.st === "error" ? "text-status-danger-text" : "text-secondary",
          )}
        >
          {props.integration.detail}
        </span>
      </Show>
      <div class="flex flex-wrap gap-2 self-start">
        <Button
          size={28}
          variant={row.connected() ? "secondary" : "primary"}
          class={cx(row.connected() && "font-semibold!")}
          onClick={row.click}
        >
          {row.status().button}
        </Button>
        <Show when={row.testable()}>
          <TestButton integration={props.integration} fold={props.fold} />
        </Show>
      </div>
      <Show when={row.open() && (props.integration.lastTest || row.showForm())}>
        <div id={bodyId} class="flex flex-col gap-2.5">
          {/* What the last connection test looked at, as a provider row shows its own. */}
          <Show when={props.integration.lastTest}>{(test) => <TestChecks test={test()} />}</Show>
          <Show when={row.showForm()}>
            <IntegrationForm integration={props.integration} edit={props.edit} />
          </Show>
        </div>
      </Show>
    </SettingsPanel>
  );
}

/** One connection as a line of the list: name and sentence, state, and the one button. */
function IntegrationRow(props: ItemProps) {
  const row = useIntegration(props.integration, props.edit, props.fold);
  const bodyId = `integration-body-${props.integration.id}`;
  return (
    <div
      data-integration={props.integration.id}
      class="flex flex-col gap-2 py-3.5 px-4 border-b border-border last:border-b-0"
    >
      <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
        <FoldToggle
          name={props.integration.name}
          open={row.open()}
          bodyId={bodyId}
          onToggle={() => props.fold.toggle(props.integration.id)}
        />
        <Icon name={props.integration.icon} size={20} class="flex-none" />
        <ItemText
          basis={220}
          tight
          title={props.integration.name}
          description={
            <Show when={row.open() && props.integration.detail}>
              <span class={cx(props.integration.st === "error" && "text-status-danger-text")}>
                {props.integration.detail}
              </span>
            </Show>
          }
        />
        <span class="w-32 flex-none">
          <IconLabel
            icon={row.status().icon}
            size={12}
            class={cx("text-caption font-semibold", row.status().textClass)}
          >
            {row.status().label}
          </IconLabel>
        </span>
        <span class="w-44 flex-none flex justify-end gap-2">
          <Show when={row.testable()}>
            <TestButton integration={props.integration} fold={props.fold} />
          </Show>
          <Button
            size={28}
            variant={row.connected() ? "secondary" : "primary"}
            class={cx(row.connected() && "font-semibold!")}
            onClick={row.click}
          >
            {row.status().button}
          </Button>
        </span>
      </div>
      <Show when={row.open() && (props.integration.lastTest || row.showForm())}>
        <div id={bodyId} class="flex flex-col gap-2 pl-14">
          <Show when={props.integration.lastTest}>{(test) => <TestChecks test={test()} />}</Show>
          <Show when={row.showForm()}>
            <IntegrationForm integration={props.integration} edit={props.edit} />
          </Show>
        </div>
      </Show>
    </div>
  );
}

/** The two layouts as icon buttons: a list, or a grid of cards. */
function ViewToggle(props: { view: IntegrationsView; onChange: (view: IntegrationsView) => void }) {
  const choice = (view: IntegrationsView, icon: IconNameInput, label: string) => (
    <IconButton
      variant={props.view === view ? "outline" : "ghost"}
      icon={icon}
      label={label}
      title={label}
      aria-pressed={props.view === view}
      onClick={() => props.onChange(view)}
    />
  );
  return (
    <fieldset class="m-0 flex min-w-0 gap-0.5 border-0 p-0">
      <legend class="sr-only">Integrations layout</legend>
      {choice("list", "list", "List view")}
      {choice("cards", "columns-2", "Card view")}
    </fieldset>
  );
}

/** Integrations: every service with its state and a Connect, Manage, or Reconnect button, as a list or as cards, each foldable. */
export function IntegrationsSection() {
  const edit = createEditState();
  const fold = createFold();
  const [view, setView] = createSignal<IntegrationsView>(readIntegrationsView());
  const choose = (next: IntegrationsView): void => {
    setView(next);
    writeIntegrationsView(next);
  };
  return (
    <SettingsSection title="Integrations" actions={<ViewToggle view={view()} onChange={choose} />}>
      <Show
        when={view() === "cards"}
        fallback={
          <SettingsPanel list data-view="list">
            <For each={M.S.integrations}>
              {(integration) => (
                <IntegrationRow integration={integration} edit={edit} fold={fold} />
              )}
            </For>
          </SettingsPanel>
        }
      >
        <div data-view="cards" class={GRID_MIN_240}>
          <For each={M.S.integrations}>
            {(integration) => <IntegrationCard integration={integration} edit={edit} fold={fold} />}
          </For>
        </div>
      </Show>
    </SettingsSection>
  );
}
