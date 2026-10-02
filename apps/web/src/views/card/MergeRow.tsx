import { Button } from "@marshal/ui";
import { Show } from "solid-js";
import type { Card, CardView } from "~/mock";
import { retryMerge } from "~/sync/integration-flow";
import { isMergeStop } from "./merge-model";
import { WorktreeMenu } from "./WorktreeMenu";

export interface MergeRowProps {
  card: Card;
  c: CardView;
}

/**
 * The merge's own line under the meta: the Integrator's note while a merge runs, the worktree menu
 * once the card has a worktree, and Retry when the merge queue stopped the card for a person.
 */
export function MergeRow(props: MergeRowProps) {
  return (
    <Show when={props.c.mergeNote || props.card.worktree || isMergeStop(props.card)}>
      <div class="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <Show when={props.c.mergeNote}>
          <span class="text-small leading-4.5 text-secondary">{props.c.mergeNote}</span>
        </Show>
        <Show when={props.card.worktree}>
          {(path) => <WorktreeMenu id={props.card.id} path={path()} />}
        </Show>
        <Show when={isMergeStop(props.card)}>
          <Button size={28} icon="repeat" onClick={() => void retryMerge(props.card.id)}>
            Retry merge
          </Button>
        </Show>
      </div>
    </Show>
  );
}
