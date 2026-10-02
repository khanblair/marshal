import { Button, Callout, Icon, IconLabel, StatusLabel } from "@marshal/ui";
import { createSignal, Match, Show, Switch } from "solid-js";
import type { MergeFlow } from "~/data/mappers/integration";
import { M } from "~/mock";
import { pauseMerging, resumeMerging } from "~/sync/integration-flow";
import { aheadLabel, integratorChatOf } from "./integration-model";

/** Opens the project's Integrator chat when the store has one, and the chat view either way. */
function openIntegratorChat(projectId: string): void {
  const chat = integratorChatOf(M.S.chats[projectId]);
  M.go("project", projectId, "chat");
  if (chat) M.openChat(projectId, chat.id);
}

/** What the Integrator is doing, in plain words, with the color of the state it is in. */
function StateLine(props: { flow: MergeFlow }) {
  return (
    <span data-integrator-state={props.flow.state} class="inline-flex items-center gap-1">
      <Switch fallback={<span class="font-semibold text-secondary">{props.flow.stateLabel}</span>}>
        <Match when={props.flow.state === "merging"}>
          <StatusLabel state="merging">{props.flow.stateLabel}</StatusLabel>
        </Match>
        <Match when={props.flow.state === "waiting"}>
          <StatusLabel state="needs">{props.flow.stateLabel}</StatusLabel>
        </Match>
        <Match when={props.flow.state === "paused"}>
          <IconLabel icon="pause" class="font-semibold text-secondary">
            {props.flow.stateLabel}
          </IconLabel>
        </Match>
      </Switch>
    </span>
  );
}

/**
 * The top of the Integration view: where finished cards land, the Integrator's own branch and how
 * far ahead it is, what it is doing, why it stopped when it did, and the two things to press.
 */
export function IntegrationHeader(props: { projectId: string; flow: MergeFlow }) {
  const [busy, setBusy] = createSignal(false);
  const paused = () => props.flow.state === "paused";
  async function toggle(): Promise<void> {
    setBusy(true);
    try {
      await (paused() ? resumeMerging(props.projectId) : pauseMerging(props.projectId));
    } finally {
      setBusy(false);
    }
  }
  return (
    <header class="flex flex-col gap-3 rounded-md border border-border bg-surface p-4">
      <div class="flex items-center gap-2">
        <Icon name="git-merge" size={20} />
        <h2 class="m-0 min-w-0 text-view-title leading-7 font-bold">
          Merging into <span class="font-mono">{props.flow.target}</span>
        </h2>
      </div>
      <div class="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-small text-secondary">
        <IconLabel icon="git-branch" title="The Integrator's own branch">
          <span class="font-mono">{props.flow.integratorBranch}</span>
        </IconLabel>
        <span
          title={`Commits that ${props.flow.integratorBranch} has and ${props.flow.target} does not yet`}
        >
          {aheadLabel(props.flow.aheadBy)}
        </span>
        <StateLine flow={props.flow} />
      </div>
      <Show when={props.flow.message}>
        <Callout icon="triangle-alert">{props.flow.message}</Callout>
      </Show>
      <div class="flex flex-wrap gap-2">
        <Button icon={paused() ? "play" : "pause"} disabled={busy()} onClick={() => void toggle()}>
          {paused() ? "Resume merging" : "Pause merging"}
        </Button>
        <Button icon="message-circle" onClick={() => openIntegratorChat(props.projectId)}>
          Open Integrator chat
        </Button>
      </div>
    </header>
  );
}
