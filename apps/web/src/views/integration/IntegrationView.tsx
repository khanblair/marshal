import { Button, Callout, EmptyState } from "@marshal/ui";
import { createMemo, For, Match, Show, Switch } from "solid-js";
import type { MergeFlow } from "~/data/mappers/integration";
import { M } from "~/mock";
import { reloadMergeFlow } from "~/sync/integration-flow";
import { DeliveredList } from "./DeliveredList";
import { IntegrationHeader } from "./IntegrationHeader";
import { NeedsYouLane, QueueLane } from "./IntegrationLanes";
import {
  ACTIVE_LANES,
  NOTHING_WAITING,
  nothingWaiting,
  stoppedCards,
  storeCardOf,
} from "./integration-model";

/** What the Integrator is merging for a project, and what it delivered, drawn from the cards. */
function Flow(props: { projectId: string; flow: MergeFlow; error: string }) {
  const cardOf = (daemonId: string) => storeCardOf(M.S.cards, props.projectId, daemonId);
  const stopped = createMemo(() => stoppedCards(M.S.cards, props.projectId, props.flow));
  const quiet = () => nothingWaiting(props.flow, stopped());
  return (
    <>
      <IntegrationHeader projectId={props.projectId} flow={props.flow} />
      <Show when={props.error}>
        <Callout tone="neutral" class="items-center">
          <span class="flex-1">{props.error}</span>
          <Button size={28} onClick={() => void reloadMergeFlow(props.projectId)}>
            Try again
          </Button>
        </Callout>
      </Show>
      <Show when={quiet()}>
        <p class="m-0 text-secondary">{NOTHING_WAITING}</p>
      </Show>
      <For each={ACTIVE_LANES}>
        {(lane) => (
          <Show when={props.flow.lanes[lane.phase].length > 0}>
            <QueueLane
              title={lane.title}
              icon={lane.icon}
              items={props.flow.lanes[lane.phase]}
              cardOf={cardOf}
            />
          </Show>
        )}
      </For>
      <Show when={stopped().length > 0}>
        <NeedsYouLane cards={stopped()} />
      </Show>
      <Show when={props.flow.delivered.length > 0}>
        <DeliveredList items={props.flow.delivered} cardOf={cardOf} />
      </Show>
    </>
  );
}

const Nothing = () => <EmptyState icon="git-merge">{NOTHING_WAITING}</EmptyState>;

/**
 * The Integration tab: a lens on the project's own cards. It shows where the Integrator is taking
 * them (waiting, resolving conflicts, testing, landing in your folder), the ones that stopped and
 * need you, and what it delivered. The daemon owns the queue, so a store with no daemon, and a
 * daemon with no merge queue, show the plain empty state.
 */
export function IntegrationView() {
  const projectId = () => M.S.route.pid ?? "";
  const slot = () => M.S.integration?.[projectId()];
  return (
    <div data-view="integration" class="@container absolute inset-0 overflow-auto bg-canvas">
      <div class="mx-auto flex min-h-full w-full max-w-[896px] flex-col gap-5 p-4 @min-[512px]:p-6">
        <Switch fallback={<Nothing />}>
          <Match when={slot()?.flow}>
            {(flow) => <Flow projectId={projectId()} flow={flow()} error={slot()?.error ?? ""} />}
          </Match>
          <Match when={slot()?.loading}>
            <p role="status" class="m-0 text-secondary">
              Reading the merge queue.
            </p>
          </Match>
          <Match when={slot()?.error}>
            <Callout tone="neutral" class="items-center">
              <span class="flex-1">{slot()?.error}</span>
              <Button size={28} onClick={() => void reloadMergeFlow(projectId())}>
                Try again
              </Button>
            </Callout>
          </Match>
        </Switch>
      </div>
    </div>
  );
}
