import { Field, Icon, Input } from "@marshal/ui";
import { For } from "solid-js";
import { FOUND_AGENTS, KEY_FIELDS } from "./onboarding-data";
import { StatusCheck } from "./StatusCheck";
import { StepIntro } from "./StepIntro";
import type { StepProps } from "./StepProps";

/** Screen 3: the agents found on this computer, and optional provider keys. */
export function AgentsStep(props: StepProps) {
  return (
    <>
      <StepIntro>
        Marshal found these agents on this computer. Add API keys to use the built-in agent with any
        provider. You can do this later in Settings.
      </StepIntro>
      <ul class="m-0 p-0 list-none border-t border-border">
        <For each={FOUND_AGENTS}>
          {(agent) => (
            <li class="flex items-center gap-2.5 py-2.5 border-b border-border">
              <Icon name="terminal-square" size={16} />
              <span class="flex-1 font-semibold">{agent.name}</span>
              <code class="font-mono text-caption text-secondary">{agent.version}</code>
              <StatusCheck>Found</StatusCheck>
            </li>
          )}
        </For>
      </ul>
      <div class="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3">
        <For each={KEY_FIELDS}>
          {(field) => (
            <Field label={field.label}>
              <Input
                mono
                type="password"
                autocomplete="off"
                value={props.draft.keys[field.id]}
                onInput={(e) => props.setDraft("keys", field.id, e.currentTarget.value)}
                placeholder="Optional"
              />
            </Field>
          )}
        </For>
      </div>
      <span class="text-small text-secondary">Keys go to your system keychain.</span>
    </>
  );
}
