import { Button, Icon } from "@marshal/ui";
import { createEffect, createSignal, For, type JSX, Show } from "solid-js";
import type { ProviderTest } from "~/data/mappers/providers";
import { M } from "~/mock";
import { TestChecks } from "./TestChecks";

/*
 * The row of an agent or tool in a list, with its Test button and its details, shared by Onboarding's
 * agent step and Settings' Agents section. A row starts closed, and opens itself while its test runs.
 */

/** The test of one row: idle, running (a test can take up to a minute), or the answer. */
export function createTester(id: () => string) {
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

export type Tester = ReturnType<typeof createTester>;

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
export function AgentListRow(props: {
  /** Classes for the row itself, such as its padding in a panel. */
  class?: string;
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
    <li
      class={`flex flex-wrap items-center gap-x-2.5 gap-y-1 py-2.5 border-b border-border ${props.class ?? ""}`}
    >
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

export const detail = (text: string): JSX.Element => (
  <span class="basis-full text-small leading-4.5 text-secondary">{text}</span>
);
