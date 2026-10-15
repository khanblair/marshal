import type { CardOverlap } from "@marshal/protocol";
import { Callout } from "@marshal/ui";
import { For, Show } from "solid-js";

/**
 * The files that more than one unfinished card has changed. A merge would conflict on each of them,
 * so they are shown while the cards are still working, with the cards that share them, and the
 * cards' own agents are told the same thing at the start of their turns.
 */
export function OverlapList(props: { overlaps: readonly CardOverlap[] }) {
  return (
    <Show when={props.overlaps.length > 0}>
      <section aria-label="Cards touching the same files" class="flex flex-col gap-2">
        <Callout icon="triangle-alert">
          {props.overlaps.length === 1
            ? "Two cards have changed the same file. Their merge would conflict."
            : `${props.overlaps.length} files have been changed by more than one card. Their merges would conflict.`}
        </Callout>
        <ul class="m-0 flex list-none flex-col gap-1 p-0">
          <For each={props.overlaps}>
            {(overlap) => (
              <li class="flex flex-wrap items-baseline gap-x-3 rounded-md border border-border bg-surface px-3 py-2">
                <span class="font-mono text-small">{overlap.path}</span>
                <span class="text-small text-secondary">{overlap.cards.join(", ")}</span>
              </li>
            )}
          </For>
        </ul>
      </section>
    </Show>
  );
}
