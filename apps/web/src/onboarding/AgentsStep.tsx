import { type Agent, AgentKindBuiltin, type AgentTool } from "@marshal/protocol";
import { Button, Icon } from "@marshal/ui";
import { createEffect, createSignal, For, type JSX, Show } from "solid-js";
import type { ProviderTest } from "~/data/mappers/providers";
import { M } from "~/mock";
import { TestChecks } from "~/views/settings/TestChecks";
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

/** The test of one row: idle, running (a test can take up to a minute), or the answer. */
function createTester(id: () => string) {
  const [running, setRunning] = createSignal(false);
  const [result, setResult] = createSignal<ProviderTest | null>(null);
  const run = async (): Promise<void> => {
    setRunning(true);
    setResult(null);
    setResult(await M.testAgent(id()));
    setRunning(false);
  };
  return { running, result, run };
}

type Tester = ReturnType<typeof createTester>;

/** The Test button of a row, and what the last test found under it. */
function TestButton(props: { tester: Tester; label: string }) {
  return (
    <Button
      size={28}
      class="text-small"
      icon="play"
      disabled={props.tester.running()}
      aria-label={`Test ${props.label}`}
      onClick={() => void props.tester.run()}
    >
      {props.tester.running() ? "Testing…" : "Test"}
    </Button>
  );
}

function TestResultView(props: { tester: Tester }): JSX.Element {
  return (
    <Show
      when={props.tester.result()}
      fallback={
        <Show when={props.tester.running()}>
          <span class="basis-full text-small text-secondary">
            Starting it and asking it to introduce itself. This can take up to a minute.
          </span>
        </Show>
      }
    >
      {(result) => (
        <div class="basis-full flex flex-col gap-1">
          <span
            class={
              result().ok
                ? "text-small font-semibold text-status-working-text"
                : "text-small font-semibold text-status-danger-text"
            }
          >
            {result().ok ? "Test passed" : "Test failed"}
          </span>
          <TestChecks test={result()} />
          <For each={result().checks.filter((check) => check.fix)}>
            {(check) => <span class="text-small text-secondary">What to do: {check.fix}</span>}
          </For>
        </div>
      )}
    </Show>
  );
}

/**
 * One row: the name, the version, a status, Test, and a chevron that shows or hides the details
 * under it. Rows start closed so the list reads clean, and a row opens itself when its test starts,
 * so the answer is never hidden from the person who asked for it.
 */
function AgentListRow(props: {
  name: string;
  version: string;
  status: JSX.Element;
  tester: Tester;
  /** Set when there is something under the row to show or hide, other than a test's answer. */
  details?: JSX.Element;
}) {
  // Null leaves it to the row: closed, until a test is asked for. A press of the chevron decides.
  const [chosen, setChosen] = createSignal<boolean | null>(null);
  createEffect(() => {
    if (props.tester.running()) setChosen(null);
  });
  const shown = () => chosen() ?? (props.tester.running() || props.tester.result() !== null);
  const hasDetails = () =>
    !!props.details || props.tester.result() !== null || props.tester.running();
  return (
    <li class="flex flex-wrap items-center gap-x-2.5 gap-y-1 py-2.5 border-b border-border">
      <Icon name="terminal-square" size={16} />
      <span class="flex-1 font-semibold">{props.name}</span>
      <Show when={props.version}>
        <code class="font-mono text-caption text-secondary">{props.version}</code>
      </Show>
      {props.status}
      <TestButton tester={props.tester} label={props.name} />
      <button
        type="button"
        class="inline-flex size-7 items-center justify-center rounded-sm border-none bg-transparent text-secondary hover:bg-surface-hover"
        aria-expanded={shown()}
        aria-label={`${shown() ? "Hide" : "Show"} details of ${props.name}`}
        disabled={!hasDetails()}
        onClick={() => setChosen(!shown())}
      >
        <Icon name={shown() ? "chevron-down" : "chevron-right"} size={16} />
      </button>
      <Show when={shown()}>
        {props.details}
        <TestResultView tester={props.tester} />
      </Show>
    </li>
  );
}

const detail = (text: string): JSX.Element => (
  <span class="basis-full text-small leading-4.5 text-secondary">{text}</span>
);

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
