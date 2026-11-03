import { Button, Icon } from "@marshal/ui";
import { Index, Show } from "solid-js";
import type { Checkpoint } from "./activity-model";

export interface CheckpointListProps {
  checkpoints: readonly Checkpoint[];
  allowRestore?: boolean;
}

/** Restore points of the session, each with a Restore button that asks first. */
export function CheckpointList(props: CheckpointListProps) {
  return (
    <section aria-labelledby="h-cp" class="p-4">
      <h3 id="h-cp" class="m-0 mb-2 text-subtitle leading-5.5 font-semibold">
        Checkpoints
      </h3>
      <Show when={props.allowRestore === false}>
        <p class="m-0 mb-2 text-small text-secondary">
          Snapshots are kept for recovery. Restoring one card automatically could overwrite other
          work in the shared project folder.
        </p>
      </Show>
      <Index each={props.checkpoints}>
        {(checkpoint) => (
          <div class="flex items-center gap-2.5 py-2 border-b border-border">
            <Icon name="history" size={14} />
            <span class="flex-1 min-w-0 flex flex-col">
              <span>{checkpoint().label}</span>
              <span class="font-mono text-caption text-secondary">{checkpoint().ref}</span>
            </span>
            <span class="text-caption text-muted">{checkpoint().when}</span>
            <Show when={props.allowRestore !== false}>
              <Button size={28} onClick={() => checkpoint().restore()}>
                Restore
              </Button>
            </Show>
          </div>
        )}
      </Index>
    </section>
  );
}
