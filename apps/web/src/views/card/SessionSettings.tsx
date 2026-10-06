import { Field, Icon, Select, type SelectOption } from "@marshal/ui";
import { For, Index, Show } from "solid-js";
import { AgentNotes } from "~/features/agents/AgentNotes";
import { agentNotes, agentSelectOptions, modelOptions } from "~/features/agents/agent-picker";
import { type Card, M } from "~/mock";
import type { Panel } from "./panel-state";

export interface SessionSettingsProps {
  card: Card;
  panel: Panel;
}

const AGENT_NOTES_ID = "card-agent-notes";

/** The card settings the design lets you change from the panel. */
type SettingKey = "agent" | "role" | "model" | "think" | "perm";

interface Setting {
  key: SettingKey;
  label: string;
  value: string;
  options: readonly (SelectOption | string)[];
}

function settingsOf(card: Card): Setting[] {
  const models = M.AGENTS[card.agent]?.models ?? [];
  const settings: Setting[] = [
    {
      key: "agent",
      label: "Agent",
      value: card.agent,
      options: agentSelectOptions(M.agentOptions(), card.agent),
    },
    { key: "role", label: "Role", value: card.role, options: M.ROLE_NAMES },
    { key: "model", label: "Model", value: card.model, options: modelOptions(models, card.model) },
  ];
  if (M.thinkSupported(card.model)) {
    settings.push({
      key: "think",
      label: "Thinking mode",
      value: card.think || "Medium",
      options: M.THINK,
    });
  }
  settings.push({ key: "perm", label: "Permission mode", value: card.perm, options: M.PERMS });
  return settings;
}

/** The collapsed line: every setting's value, joined, with the permission mode in red when the
 * card is in bypass, and a warning glyph when the agent has a note (an untested version, or
 * another agent not installed). Click anywhere on it, or the chevron, to see the full settings. */
function SettingsSummary(props: SessionSettingsProps & { onExpand: () => void }) {
  const settings = () => settingsOf(props.card);
  const hasNotes = () => agentNotes(M.agentOptions(), props.card.agent).length > 0;
  return (
    <button
      type="button"
      onClick={props.onExpand}
      aria-expanded={false}
      class="flex items-center gap-1.5 -mx-1 px-1 py-0.5 rounded-sm text-small text-secondary text-left hover:bg-surface-hover"
    >
      <Icon name="chevron-right" size={14} class="flex-none text-muted" />
      <span class="flex-1 min-w-0 truncate">
        <For each={settings()}>
          {(setting, i) => (
            <>
              <Show when={i() > 0}> · </Show>
              <span
                class={
                  setting.key === "perm" && props.card.bypass
                    ? "text-status-danger-text font-semibold"
                    : undefined
                }
              >
                {setting.value}
              </span>
            </>
          )}
        </For>
      </span>
      <Show when={hasNotes()}>
        <Icon name="triangle-alert" size={14} class="flex-none text-status-needs-you-text" />
      </Show>
    </button>
  );
}

/** Agent, role, model, thinking mode, and permission mode, collapsed to one summary line by
 * default so a card you have no reason to reconfigure opens calmer. Expands to the full editable
 * grid, its notes, and the "changes take effect" caption on click, and collapses back the same way. */
export function SessionSettings(props: SessionSettingsProps) {
  return (
    <Show
      when={props.panel.state.settingsExpanded}
      fallback={
        <SettingsSummary
          card={props.card}
          panel={props.panel}
          onExpand={() => props.panel.set({ settingsExpanded: true })}
        />
      }
    >
      <div class="flex flex-col gap-2">
        <div class="grid gap-2 grid-cols-[repeat(auto-fit,minmax(118px,1fr))]">
          <Index each={settingsOf(props.card)}>
            {(setting) => (
              <Field label={setting().label} compact>
                <Select
                  options={setting().options}
                  value={setting().value}
                  danger={setting().key === "perm" && props.card.bypass}
                  aria-describedby={setting().key === "agent" ? AGENT_NOTES_ID : undefined}
                  onChange={(e) =>
                    M.setSetting(props.card.id, setting().key, e.currentTarget.value)
                  }
                  class={`h-7! phone:h-(--control-h)! px-1.5! text-small min-w-0`}
                />
              </Field>
            )}
          </Index>
        </div>
        <AgentNotes id={AGENT_NOTES_ID} selected={props.card.agent} />
        <div class="flex items-center justify-between gap-2">
          <span class="text-caption leading-4 text-secondary">
            Changes take effect on the next turn.
          </span>
          <button
            type="button"
            onClick={() => props.panel.set({ settingsExpanded: false })}
            class="flex-none text-caption leading-4 text-secondary hover:text-primary"
          >
            Hide
          </button>
        </div>
      </div>
    </Show>
  );
}
