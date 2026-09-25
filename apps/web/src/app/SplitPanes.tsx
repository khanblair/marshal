import { IconButton, Select } from "@marshal/ui";
import { Index } from "solid-js";
import { Dynamic } from "solid-js/web";
import { M, type ViewKey } from "~/mock";
import { isProject, maxPanes } from "./shell-layout";
import { VIEW_COMPONENTS } from "./view-components";

const VIEW_OPTIONS = M.VIEWS.map((v) => ({ value: v.key, label: v.label }));

/** The views shown beside the main one: none on phones, and none while the card fills the screen. */
const paneViews = (): ViewKey[] =>
  isProject() && !M.S.detailExpanded ? M.S.split.slice(0, maxPanes()) : [];

function setPaneView(index: number, value: string): void {
  const view = M.VIEWS.find((v) => v.key === value);
  if (view) M.S.split[index] = view.key;
}

/** Extra views beside the main one (desktop up to 3, tablet 1), each with a view picker and a close button. */
export function SplitPanes() {
  return (
    <Index each={paneViews()}>
      {(view, index) => (
        <section
          aria-label={`Pane ${index + 2}`}
          class="flex-1 min-w-[300px] flex flex-col border-l-4 border-border"
        >
          <div class="flex-none flex items-center gap-2 h-10 pl-3 pr-2 border-b border-border bg-surface-sunken">
            <Select
              aria-label="View in this pane"
              class="h-7! px-1.5! text-small font-semibold"
              options={VIEW_OPTIONS}
              value={view()}
              onChange={(event) => setPaneView(index, event.currentTarget.value)}
            />
            <span class="flex-1" />
            <IconButton
              label="Close pane"
              icon="x"
              iconSize={14}
              tone="default"
              onClick={() => M.S.split.splice(index, 1)}
            />
          </div>
          <div class="flex-1 min-h-0 relative overflow-hidden">
            <Dynamic component={VIEW_COMPONENTS[view()]} />
          </div>
        </section>
      )}
    </Index>
  );
}
