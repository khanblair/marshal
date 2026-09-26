import { DiffStat, ErrorState, Skeleton, SkeletonGroup } from "@marshal/ui";
import { For, Show } from "solid-js";
import type { DiffFile } from "~/mock";

export interface DiffSummaryProps {
  files: readonly DiffFile[];
  /** The list is being fetched. */
  loading: boolean;
  /** Why the list could not be fetched, when it could not. */
  error: unknown;
  allExpanded: boolean;
  onRetry: () => void;
  /** The Expand all or Collapse all button: true to open every file. */
  onToggleAll: (expand: boolean) => void;
}

const PLACEHOLDER_FILES = [0, 1, 2];
/** A file's closed row: `min-h-9` inside a 1 px border. */
const PLACEHOLDER_PX = 38;

const errorText = (error: unknown): string =>
  error instanceof Error ? error.message : "Marshal could not load this card's changes.";

/**
 * What sits above the changed files: the count and the Expand all button, or, while there are no
 * files to show, why: loading, failed, or nothing changed yet.
 */
export function DiffSummary(props: DiffSummaryProps) {
  const none = () => props.files.length === 0;
  const added = () => props.files.reduce((sum, file) => sum + file.add, 0);
  const removed = () => props.files.reduce((sum, file) => sum + file.del, 0);
  return (
    <>
      <Show when={props.error && none()}>
        <ErrorState message={errorText(props.error)} onRetry={() => props.onRetry()} />
      </Show>
      <Show when={props.loading && none() && !props.error}>
        <SkeletonGroup label="Loading changes" class="flex flex-col gap-2.5">
          <For each={PLACEHOLDER_FILES}>
            {() => <Skeleton height={PLACEHOLDER_PX} class="rounded-md!" />}
          </For>
        </SkeletonGroup>
      </Show>
      <Show when={none() && !props.loading && !props.error}>
        <div class="py-8 px-2 text-center text-secondary">
          No changes yet. The diff fills in as the agent edits files.
        </div>
      </Show>
      <Show when={!none()}>
        <div class="flex items-center gap-2.5 text-small">
          <span class="font-semibold">
            {props.files.length}
            {props.files.length === 1 ? " file changed" : " files changed"}
          </span>
          <DiffStat added={added().toLocaleString()} removed={removed()} />
          <span class="flex-1" />
          <button
            type="button"
            onClick={() => props.onToggleAll(!props.allExpanded)}
            class="h-7 px-2 border-none rounded-sm bg-transparent text-secondary text-small hover:bg-surface-hover"
          >
            {props.allExpanded ? "Collapse all" : "Expand all"}
          </button>
        </div>
      </Show>
    </>
  );
}
