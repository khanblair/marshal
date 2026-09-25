import {
  Badge,
  type BadgeProps,
  Button,
  ItemText,
  SettingsPanel,
  SettingsSection,
} from "@marshal/ui";
import { For, Show } from "solid-js";
import { M, type Provider } from "~/mock";
import type { EditState } from "./edit-state";
import { ProviderKeyForm } from "./ProviderKeyForm";

interface KeyBadge {
  label: string;
  icon: string;
  tone: BadgeProps["tone"];
}

const KEY_BADGES: Record<Provider["st"], KeyBadge> = {
  saved: { label: "Saved", icon: "check", tone: "working" },
  empty: { label: "Not set", icon: "circle-dashed", tone: "neutral" },
  invalid: { label: "Invalid key", icon: "x", tone: "danger" },
};

function editLabel(provider: Provider): string {
  if (provider.st === "empty") return "Add key";
  return provider.local ? "Change URL" : "Change key";
}

function ProviderRow(props: { provider: Provider; edit: EditState }) {
  const editing = () => props.edit.id() === props.provider.id;
  const badge = () => KEY_BADGES[props.provider.st];
  return (
    <div class="flex flex-col gap-2 py-3.5 px-4 border-b border-border">
      <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
        <ItemText
          basis={180}
          tight
          title={props.provider.name}
          description={props.provider.models}
        />
        <Badge size={22} tone={badge().tone} icon={badge().icon}>
          {badge().label}
        </Badge>
        <Show when={props.provider.masked}>
          <code class="font-mono text-caption text-secondary">{props.provider.masked}</code>
        </Show>
        <Show when={!editing()}>
          <Button size={28} onClick={() => props.edit.open(props.provider.id)}>
            {editLabel(props.provider)}
          </Button>
        </Show>
      </div>
      <Show when={props.provider.st === "invalid" && !editing()}>
        <span class="text-small leading-4.5 text-status-danger-text">{props.provider.error}</span>
      </Show>
      <Show when={editing()}>
        <ProviderKeyForm provider={props.provider} edit={props.edit} />
      </Show>
    </div>
  );
}

/** Provider keys: one row per provider with its key state, and an inline form to change the key. */
export function ProvidersSection(props: { edit: EditState }) {
  return (
    <SettingsSection
      title="Provider keys"
      description="The built-in agent uses these keys. Keys are stored in your system keychain, never in Marshal's database."
    >
      <SettingsPanel list>
        <For each={M.S.providers}>
          {(provider) => <ProviderRow provider={provider} edit={props.edit} />}
        </For>
      </SettingsPanel>
    </SettingsSection>
  );
}
