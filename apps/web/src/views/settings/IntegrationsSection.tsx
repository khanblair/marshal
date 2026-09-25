import {
  Button,
  cx,
  Icon,
  IconLabel,
  type IconNameInput,
  SettingsPanel,
  SettingsSection,
} from "@marshal/ui";
import { batch, For } from "solid-js";
import { type Integration, M } from "~/mock";
import { GRID_MIN_240 } from "./auto-fit-grid";

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

function IntegrationCard(props: { integration: Integration }) {
  const status = () => STATUSES[props.integration.st];
  const connected = () => props.integration.st === "connected";
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
      <span
        class={cx(
          "flex-1 text-small leading-4.5",
          props.integration.st === "error" ? "text-status-danger-text" : "text-secondary",
        )}
      >
        {props.integration.detail}
      </span>
      <Button
        size={28}
        variant={connected() ? "secondary" : "primary"}
        class={cx("self-start", connected() && "font-semibold!")}
        onClick={() => runIntegration(props.integration)}
      >
        {status().button}
      </Button>
    </SettingsPanel>
  );
}

/** Integrations: a card per service with its state and a Connect, Manage, or Reconnect button. */
export function IntegrationsSection() {
  return (
    <SettingsSection title="Integrations">
      <div class={GRID_MIN_240}>
        <For each={M.S.integrations}>
          {(integration) => <IntegrationCard integration={integration} />}
        </For>
      </div>
    </SettingsSection>
  );
}
