import type { Agent } from "@marshal/protocol";
import { Field, Icon, Input } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M } from "~/mock";
import { KEY_FIELDS } from "./onboarding-data";
import { StatusCheck } from "./StatusCheck";
import { StepIntro } from "./StepIntro";
import type { StepProps } from "./StepProps";

const SHARED_INTRO =
  "Add API keys to use the built-in agent with any provider. You can do this later in Settings.";

/** The sentence under an agent: how to install it when it is missing, or what to know about an untested version. */
const noteOf = (agent: Agent): string =>
  agent.status === "missing" ? agent.installHint : agent.warning;

/** One agent as the daemon reports it: found (with its version), found but untested, or not installed. */
function AgentRow(props: { agent: Agent }) {
  const missing = () => props.agent.status === "missing";
  return (
    <li class="flex flex-wrap items-center gap-x-2.5 gap-y-1 py-2.5 border-b border-border">
      <Icon name="terminal-square" size={16} />
      <span class="flex-1 font-semibold">{props.agent.name}</span>
      <Show when={props.agent.version}>
        <code class="font-mono text-caption text-secondary">{props.agent.version}</code>
      </Show>
      <Show
        when={!missing()}
        fallback={<span class="text-small font-semibold text-secondary">Not installed</span>}
      >
        <StatusCheck>Found</StatusCheck>
      </Show>
      <Show when={noteOf(props.agent)}>
        {(note) => <span class="basis-full text-small leading-4.5 text-secondary">{note()}</span>}
      </Show>
    </li>
  );
}

/**
 * Screen 3: the agents Marshal looked for on this computer, as the daemon reports them (the
 * built-in agent is Marshal's own, not something found), and optional provider keys.
 */
export function AgentsStep(props: StepProps) {
  const anyMissing = () => M.S.agents.some((agent) => agent.status === "missing");
  return (
    <>
      <StepIntro>
        {anyMissing()
          ? "Marshal looked for these agents on this computer."
          : "Marshal found these agents on this computer."}{" "}
        {SHARED_INTRO}
      </StepIntro>
      <ul class="m-0 p-0 list-none border-t border-border">
        <For each={M.S.agents}>{(agent) => <AgentRow agent={agent} />}</For>
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
