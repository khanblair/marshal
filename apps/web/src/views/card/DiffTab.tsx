import { DiffStat } from "@marshal/ui";
import { Index, Show } from "solid-js";
import { type Card, M } from "~/mock";
import { DiffFileView } from "./DiffFileView";
import { allOpen, openPaths, toggled } from "./diff-model";
import type { Panel } from "./panel-state";

export interface DiffTabProps {
  card: Card;
  panel: Panel;
}

/** The Diff tab: a summary line, then every changed file, expandable. */
export function DiffTab(props: DiffTabProps) {
  const diff = () => M.diffFor(props.card);
  const open = () => openPaths(diff(), props.panel.state.openFiles);
  const added = () => diff().reduce((sum, file) => sum + file.add, 0);
  const removed = () => diff().reduce((sum, file) => sum + file.del, 0);
  const allExpanded = () => open().size === diff().length;
  return (
    <div class="flex-1 min-h-0 overflow-auto py-3 px-4 flex flex-col gap-2.5">
      <Show when={diff().length === 0}>
        <div class="py-8 px-2 text-center text-secondary">
          No changes yet. The diff fills in as the agent edits files.
        </div>
      </Show>
      <Show when={diff().length > 0}>
        <div class="flex items-center gap-2.5 text-small">
          <span class="font-semibold">
            {diff().length}
            {diff().length === 1 ? " file changed" : " files changed"}
          </span>
          <DiffStat added={added().toLocaleString()} removed={removed()} />
          <span class="flex-1" />
          <button
            type="button"
            onClick={() => props.panel.set({ openFiles: allExpanded() ? [] : allOpen(diff()) })}
            class="h-7 px-2 border-none rounded-sm bg-transparent text-secondary text-small hover:bg-surface-hover"
          >
            {allExpanded() ? "Collapse all" : "Expand all"}
          </button>
        </div>
      </Show>
      <Index each={diff()}>
        {(file) => (
          <DiffFileView
            file={file()}
            open={open().has(file().path)}
            loaded={!!props.panel.state.loaded[file().path]}
            onToggle={() => props.panel.set({ openFiles: toggled(open(), file().path) })}
            onLoad={() =>
              props.panel.set({ loaded: { ...props.panel.state.loaded, [file().path]: true } })
            }
          />
        )}
      </Index>
    </div>
  );
}
