import { Show } from "solid-js";
import { Dynamic } from "solid-js/web";
import { M } from "~/mock";
import { DetailPanel } from "./DetailPanel";
import { SplitPanes } from "./SplitPanes";
import { detailOpen, isPhone } from "./shell-layout";
import { routeComponent } from "./view-components";

/** Phones and the expanded card panel take the whole row, so the main view steps aside. */
const showMain = (): boolean => !(detailOpen() && (isPhone() || M.S.detailExpanded));

/**
 * The row under the bars: the current route's view, the split panes beside it, and
 * the open card's panel. Port of the design's main row.
 */
export function Workspace() {
  return (
    <div class="flex-1 min-h-0 flex relative">
      <Show when={showMain()}>
        <main class="flex-1 min-w-0 min-h-0 relative overflow-hidden">
          <Dynamic component={routeComponent()} />
        </main>
        <SplitPanes />
      </Show>
      <DetailPanel />
    </div>
  );
}
