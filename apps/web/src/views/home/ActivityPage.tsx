import { createEffect, createMemo, createSignal, For, Show } from "solid-js";
import { M } from "~/mock";
import type { FeedItem } from "~/mock/types";
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

  // `M.loadActivityPage` is the store's cached `entries()` while S20 is still on the mock (a plain
  // filter, one page, nothing to load further) and the daemon's own paged, server-filtered stream
  // once it is switched. `pages` is null until the fetch for the current filter answers, so the
  // first paint, and every filter change, shows `entries()` and never an empty list while it loads.
  // `epoch` drops an answer that is no longer for the filter that asked.
  const [pages, setPages] = createSignal<FeedItem[] | null>(null);
  const [cursor, setCursor] = createSignal("");
  const [loadingMore, setLoadingMore] = createSignal(false);
  let epoch = 0;
  const query = () => ({
    kind: props.kind === "all" ? undefined : props.kind,
    project: props.projectId === "all" ? undefined : props.projectId,
  });
  createEffect(() => {
    query();
    setPages(null);
    setCursor("");
    const asked = ++epoch;
    void M.loadActivityPage(query()).then((page) => {
      if (asked !== epoch) return;
      setPages(page.items);
      setCursor(page.nextCursor);
    });
  });
  const shown = createMemo(() => pages() ?? entries());
  const loadMore = async (): Promise<void> => {
    setLoadingMore(true);
    try {
      const asked = epoch;
      const page = await M.loadActivityPage({ ...query(), cursor: cursor() || undefined });
      if (asked !== epoch) return;
      setPages((prev) => [...(prev ?? []), ...page.items]);
      setCursor(page.nextCursor);
    } finally {
      setLoadingMore(false);
    }
  };

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
      <Show when={shown().length === 0}>
        <p class="m-0 text-secondary">No activity matches these filters.</p>
      </Show>
      <FeedList entries={shown()} />
      {/* Only the daemon's own page ever carries a cursor: see M.loadActivityPage. */}
      <Show when={cursor() !== ""}>
        <button
          type="button"
          disabled={loadingMore()}
          onClick={() => void loadMore()}
          class="h-8 self-start px-3 text-small font-medium text-secondary hover:text-primary disabled:opacity-60"
        >
          {loadingMore() ? "Loading…" : "Load more"}
        </button>
      </Show>
    </>
  );
}
