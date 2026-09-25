import { Field, Select, type SelectOption } from "@marshal/ui";
import { Index } from "solid-js";
import { AgentNotes } from "~/features/agents/AgentNotes";
import { agentSelectOptions, modelOptions } from "~/features/agents/agent-picker";
import { type Card, M } from "~/mock";

export interface SessionSettingsProps {
  card: Card;
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

/** Agent, role, model, thinking mode, and permission mode. Bypass shows in red. */
export function SessionSettings(props: SessionSettingsProps) {
  return (
    <>
      <div class="grid gap-2 grid-cols-[repeat(auto-fit,minmax(118px,1fr))]">
        <Index each={settingsOf(props.card)}>
          {(setting) => (
            <Field label={setting().label} compact>
              <Select
                options={setting().options}
                value={setting().value}
                danger={setting().key === "perm" && props.card.bypass}
                aria-describedby={setting().key === "agent" ? AGENT_NOTES_ID : undefined}
                onChange={(e) => M.setSetting(props.card.id, setting().key, e.currentTarget.value)}
                class={`${M.mobile ? "h-11!" : "h-7!"} px-1.5! text-small min-w-0`}
              />
            </Field>
          )}
        </Index>
      </div>
      <AgentNotes id={AGENT_NOTES_ID} selected={props.card.agent} />
    </>
  );
}
