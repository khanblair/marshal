import { type Agent, AgentKindBuiltin, type AgentTool } from "@marshal/protocol";
import { Button } from "@marshal/ui";
import { createSignal, For, Show } from "solid-js";
import { M } from "~/mock";
import { AgentListRow, createTester, detail } from "~/views/settings/agent-rows";
import { StatusCheck } from "./StatusCheck";
import { StepIntro } from "./StepIntro";
import type { StepProps } from "./StepProps";

const SHARED_INTRO =
  "To use the built-in agent, add an API key for a provider in Settings, whenever you are ready.";

/** The sentence under an agent: how to install it when it is missing, or what to know about an untested version. */
const noteOf = (agent: Agent): string =>
  agent.status === "missing" ? agent.installHint : agent.warning;

/** How each other tool takes its work, in words: what decides whether Marshal could drive it. */
const INTERFACE_LABELS: Record<AgentTool["interface"], string> = {
  acp: "Agent Client Protocol",
  print: "Prompt mode",
  rpc: "RPC mode",
};

/** One agent as the daemon reports it: found (with its version), found but untested, or not installed. */
function AgentRow(props: { agent: Agent }) {
  const missing = () => props.agent.status === "missing";
  const tester = createTester(() => props.agent.kind);
  return (
    <AgentListRow
      name={props.agent.name}
      version={props.agent.version}
      tester={tester}
      status={
        <Show
          when={!missing()}
          fallback={<span class="text-small font-semibold text-secondary">Not installed</span>}
        >
          <StatusCheck>Found</StatusCheck>
        </Show>
      }
      details={noteOf(props.agent) ? detail(noteOf(props.agent)) : undefined}
    />
  );
}

/** Another agent program found on this computer, which Marshal cannot start for a card yet. */
function ToolRow(props: { tool: AgentTool }) {
  const tester = createTester(() => props.tool.id);
  return (
    <AgentListRow
      name={props.tool.name}
      version={props.tool.version}
      tester={tester}
      status={<StatusCheck>Found</StatusCheck>}
      details={detail(`${INTERFACE_LABELS[props.tool.interface]}. ${props.tool.note}`)}
    />
  );
}

/**
 * Screen 3: the agents Marshal looked for on this computer, as the daemon reports them, and
 * and where to add a provider key. Marshal's own agent is not one of them: the daemon lists it, but it is
 * part of Marshal rather than something it found here.
 */
export function AgentsStep(_props: StepProps) {
  const lookedFor = () => M.S.agents.filter((agent) => agent.kind !== AgentKindBuiltin);
  const anyMissing = () => lookedFor().some((agent) => agent.status === "missing");
  const [scanning, setScanning] = createSignal(false);
  const [scanned, setScanned] = createSignal(false);
  const scan = async (): Promise<void> => {
    setScanning(true);
    setScanned(await M.scanAgents());
    setScanning(false);
  };
  return (
    <>
      <StepIntro>
        {anyMissing()
          ? "Marshal looked for these agents on this computer."
          : "Marshal found these agents on this computer."}{" "}
        {SHARED_INTRO}
      </StepIntro>
      <div class="flex items-center gap-2.5">
        <Button
          size={28}
          class="text-small"
          icon="scan-search"
          disabled={scanning()}
          onClick={() => void scan()}
        >
          {scanning() ? "Scanning…" : "Scan again"}
        </Button>
        <Show when={scanned() && !scanning()}>
          <span role="status" class="text-small text-secondary">
            Scanned just now.
          </span>
        </Show>
      </div>
      <ul class="m-0 p-0 list-none border-t border-border">
        <For each={lookedFor()}>{(agent) => <AgentRow agent={agent} />}</For>
      </ul>
      <Show when={M.S.agentTools.length > 0}>
        <div class="flex flex-col gap-1">
          <span class="font-medium">Also found on this computer</span>
          <span class="text-small text-secondary">
            These run on their own, but Marshal cannot start them for a card yet.
          </span>
        </div>
        <ul class="m-0 p-0 list-none border-t border-border">
          <For each={M.S.agentTools}>{(tool) => <ToolRow tool={tool} />}</For>
        </ul>
      </Show>
    </>
  );
}
