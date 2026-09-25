import { Field, IconLabel, Select, type SelectProps } from "@marshal/ui";
import { createMemo, For } from "solid-js";
import { AgentNotes } from "~/features/agents/AgentNotes";
import { agentSelectOptions, modelOptions } from "~/features/agents/agent-picker";
import { M, type Role } from "~/mock";
import { GRID_MIN_140, GRID_MIN_170 } from "./auto-fit-grid";
import { NumberInput } from "./NumberInput";
import type { RoleDraft, RoleTextKey } from "./use-role-draft";

interface RoleSelectSpec {
  key: Extract<RoleTextKey, "agent" | "model" | "backup" | "think" | "perm">;
  label: string;
}

const ROLE_SELECTS: readonly RoleSelectSpec[] = [
  { key: "agent", label: "Agent" },
  { key: "model", label: "Model" },
  { key: "backup", label: "Backup model" },
  { key: "think", label: "Thinking mode" },
  { key: "perm", label: "Permission mode" },
];

const AGENT_NOTES_ID = "role-agent-notes";
/** Bypass is a per-card choice with its own acknowledgement, so roles cannot default to it. */
const ROLE_PERMISSIONS = M.PERMS.filter((permission) => permission !== "Bypass permissions");

/** Every model any agent in the catalog can run, once each, for the backup model. */
const allModels = (): string[] => [
  ...new Set(M.agentOptions().flatMap((agent) => agent.models.map((model) => model.id))),
];

/**
 * Agent, model, backup model, thinking mode, and permission mode, all from the daemon's catalog.
 * The models follow the agent, and thinking mode is offered only for a model that has one.
 */
export function RoleSelects(props: { draft: RoleDraft; role: () => Role }) {
  const models = createMemo(() => M.AGENTS[props.role().agent]?.models ?? []);
  const specs = () =>
    ROLE_SELECTS.filter((spec) => spec.key !== "think" || M.thinkSupported(props.role().model));
  const options = (key: RoleSelectSpec["key"]): SelectProps["options"] => {
    const role = props.role();
    if (key === "agent") return agentSelectOptions(M.agentOptions(), role.agent);
    if (key === "model") return modelOptions(models(), role.model);
    if (key === "backup") return modelOptions(allModels(), role.backup);
    return key === "think" ? M.THINK : ROLE_PERMISSIONS;
  };
  return (
    <>
      <div class={GRID_MIN_170}>
        <For each={specs()}>
          {(spec) => (
            <Field label={spec.label}>
              <Select
                options={options(spec.key)}
                value={props.role()[spec.key]}
                aria-describedby={spec.key === "agent" ? AGENT_NOTES_ID : undefined}
                onChange={(e) => props.draft.editText(spec.key, e.currentTarget.value)}
              />
            </Field>
          )}
        </For>
      </div>
      <AgentNotes id={AGENT_NOTES_ID} selected={props.role().agent} />
    </>
  );
}

/** Time, cost, and round limits. */
export function RoleLimits(props: { draft: RoleDraft; role: () => Role }) {
  return (
    <div class={GRID_MIN_140}>
      <Field label="Time limit">
        <span class="flex items-center gap-1.5">
          <NumberInput
            class="w-full"
            min="1"
            value={props.role().limits.time}
            onInput={(e) => props.draft.editLimit("time", e.currentTarget.value)}
          />
          <span class="text-secondary">min</span>
        </span>
      </Field>
      <Field label="Cost limit">
        <span class="flex items-center gap-1.5">
          <span class="text-secondary">$</span>
          <NumberInput
            class="w-full"
            min="0"
            step="0.5"
            value={props.role().limits.cost}
            onInput={(e) => props.draft.editLimit("cost", e.currentTarget.value)}
          />
        </span>
      </Field>
      <Field label="Round limit">
        <NumberInput
          min="1"
          value={props.role().limits.rounds}
          onInput={(e) => props.draft.editLimit("rounds", e.currentTarget.value)}
        />
      </Field>
    </div>
  );
}

const TAG_CLASS =
  "h-6 px-2 rounded-xs border border-border bg-surface-sunken font-mono text-caption";

/** The role's skills and MCP servers, as read-only tags. */
export function RoleTags(props: { role: () => Role }) {
  return (
    <div class="flex flex-col gap-1.5">
      <span class="font-medium">Skills and MCP servers</span>
      <div class="flex flex-wrap gap-1.5">
        <For each={props.role().skills}>
          {(skill) => (
            <IconLabel icon="sparkles" size={12} class={TAG_CLASS}>
              {skill}
            </IconLabel>
          )}
        </For>
        <For each={props.role().mcp}>
          {(server) => (
            <IconLabel icon="server" size={12} class={TAG_CLASS}>
              {server}
            </IconLabel>
          )}
        </For>
      </div>
    </div>
  );
}
