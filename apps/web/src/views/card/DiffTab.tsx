import { VirtualList } from "@marshal/ui";
import { type Accessor, createEffect, createMemo, onMount, untrack } from "solid-js";
import type { Card, DiffFile } from "~/mock";
import { DiffFileView } from "./DiffFileView";
import { DiffSummary } from "./DiffSummary";
import { createHunkLoader } from "./diff-hunks";
import type { DiffList } from "./diff-list";
import { allOpen, estimateFileHeight, FILE_GAP_PX, openPaths, toggled } from "./diff-model";
import type { Panel } from "./panel-state";

export interface DiffTabProps {
  card: Card;
  panel: Panel;
  /** The card's changed files, which the card panel keeps so the tab bar can count them too. */
  list: DiffList;
}

/**
 * The Diff tab: a summary line, then every changed file, expandable. Only the files near the
 * screen are drawn, so a diff of thousands of files stays as light as one of ten.
 */
export function DiffTab(props: DiffTabProps) {
  const diff = props.list.files;
  // The list was asked for when the card opened, and the agent may have changed files since, so
  // opening the tab asks again. The first answer may still be on its way: then it is not asked twice.
  onMount(() => {
    if (props.list.ready()) props.list.refetch();
  });
  const open = createMemo(() => openPaths(diff(), props.panel.state.openFiles));
  const loader = createHunkLoader(() => props.card, props.panel);

  const estimateHeight = (file: DiffFile): number =>
    estimateFileHeight(open().has(file.path), loader.hunksOf(file));

  const row = (file: Accessor<DiffFile>) => {
    // A small file's hunks load the moment it is open, the same way the mock's were always there;
    // a large one waits for the person to press "Load diff" (DiffFileView's own onLoad). The row
    // exists only while it is near the screen, so a diff opened all at once fetches what is seen.
    createEffect(() => {
      const current = file();
      if (open().has(current.path) && !current.large) void untrack(() => loader.load(current));
    });
    return (
      <DiffFileView
        file={{ ...file(), hunks: loader.hunksOf(file()) }}
        open={open().has(file().path)}
        loaded={loader.isLoaded(file())}
        onToggle={() => props.panel.set({ openFiles: toggled(open(), file().path) })}
        onLoad={() => void loader.load(file())}
      />
    );
  };

  return (
    <VirtualList
      class="flex-1 min-h-0 py-3 px-4 flex flex-col gap-2.5"
      aria-busy={props.list.loading()}
      label="Changed files"
      items={diff()}
      itemKey={(file) => file.path}
      estimateHeight={estimateHeight}
      gap={FILE_GAP_PX}
      before={
        <DiffSummary
          files={diff()}
          loading={props.list.loading()}
          error={props.list.error()}
          allExpanded={open().size === diff().length}
          onRetry={() => props.list.refetch()}
          onToggleAll={(expand) => props.panel.set({ openFiles: expand ? allOpen(diff()) : [] })}
        />
      }
    >
      {row}
    </VirtualList>
  );
}
