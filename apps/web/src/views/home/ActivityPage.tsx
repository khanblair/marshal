import { createMemo, For, Show } from "solid-js";
import { M } from "~/mock";
import { FeedList } from "./FeedList";

/** The kinds you can filter the activity page by: the feed's kind (or `all`) and its label. */
const KINDS: readonly (readonly [kind: string, label: string])[] = [
  ["all", "All"],
  ["merge", "Merges"],
  ["ci", "CI results"],
  ["approval", "Approvals"],
  ["plan", "Plans"],
  ["schedule", "Schedule runs"],
  ["brief", "Briefs"],
];

function KindPill(props: { label: string; selected: boolean; onSelect: () => void }) {
  return (
    <button
      type="button"
      aria-pressed={props.selected}
      onClick={() => props.onSelect()}
      class={`h-8 px-3 rounded-full border text-small font-medium ${
        props.selected
          ? "border-ink bg-ink text-on-ink"
          : "border-border-strong bg-surface text-primary"
      }`}
    >
      {props.label}
    </button>
  );
}

interface ActivityPageProps {
  /** The feed kind to show, or `all`. */
  kind: string;
  onKindChange: (kind: string) => void;
  /** The project to show, or `all`. */
  projectId: string;
}

/** The Recent activity page: kind pills over every feed entry that matches them. */
export function ActivityPage(props: ActivityPageProps) {
  const entries = createMemo(() =>
    M.S.feed.filter(
      (f) =>
        (props.kind === "all" || f.kind === props.kind) &&
        (props.projectId === "all" || f.pid === props.projectId),
    ),
  );
  return (
    <>
      {/* biome-ignore lint/a11y/useSemanticElements: the design uses a div with role group, a fieldset would add a border and legend */}
      <div role="group" aria-label="Kinds of activity" class="flex flex-wrap gap-1.5">
        <For each={KINDS}>
          {([kind, label]) => (
            <KindPill
              label={label}
              selected={props.kind === kind}
              onSelect={() => props.onKindChange(kind)}
            />
          )}
        </For>
      </div>
      <Show when={entries().length === 0}>
        <p class="m-0 text-secondary">No activity matches these filters.</p>
      </Show>
      <FeedList entries={entries()} />
    </>
  );
}
