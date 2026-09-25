import { Icon, StatusDot } from "@marshal/ui";
import { Index, Show } from "solid-js";
import { type Card, type CardView, M } from "~/mock";
import { activityRows, checkpointsFor } from "./activity-model";
import { CheckpointList } from "./CheckpointList";

export interface ActivityTabProps {
  card: Card;
  c: CardView;
}

/** The Activity tab: what is happening now, the log of actions, and restore points. */
export function ActivityTab(props: ActivityTabProps) {
  const rows = () => activityRows(M.S.act[props.card.id] ?? []);
  const checkpoints = () =>
    checkpointsFor(props.card.id, props.card.state !== "backlog", M.filesFor(props.card)[0] ?? "");
  return (
    <div class="flex-1 min-h-0 overflow-auto">
      <Show when={props.c.showDoing}>
        <div class="flex items-center gap-2 py-2.5 px-4 border-b border-border text-small">
          <StatusDot state="working" />
          <span class="font-semibold">Doing now</span>
          <span class="text-secondary">{props.c.doing}</span>
        </div>
      </Show>
      <Show when={rows().length === 0}>
        <div class="py-8 px-4 text-center text-secondary">
          No activity yet. Actions show here as the agent works.
        </div>
      </Show>
      <ol aria-live="polite" class="m-0 p-0 list-none">
        <Index each={rows()}>
          {(row) => (
            <li
              class={`flex items-start gap-2.5 py-2 px-4 border-b border-border transition-[background-color] duration-slow ease-standard ${
                row().fresh ? "bg-surface-selected" : "bg-transparent"
              }`}
            >
              <span class="inline-flex mt-0.5 text-secondary">
                <Icon name={row().icon} size={14} />
              </span>
              <span class="flex-1 min-w-0 flex flex-col gap-px">
                <span
                  class={`wrap-anywhere ${row().mono ? "font-mono text-caption" : "font-sans text-small"}`}
                >
                  {row().text}
                </span>
                <Show when={row().result}>
                  <span
                    class={`inline-flex items-center gap-1 text-caption leading-4 font-semibold ${row().resultClass}`}
                  >
                    <Icon name={row().resultIcon} size={12} />
                    {row().result}
                  </span>
                </Show>
              </span>
              <span title={row().full} class="flex-none text-caption leading-5 text-muted">
                {row().when}
              </span>
            </li>
          )}
        </Index>
      </ol>
      <CheckpointList checkpoints={checkpoints()} />
    </div>
  );
}
