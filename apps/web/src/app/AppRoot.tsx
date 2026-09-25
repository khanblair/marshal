import { Show } from "solid-js";
import { Tour } from "~/features/tutorial/Tour";
import { M } from "~/mock";
import { Onboarding } from "~/onboarding/Onboarding";
import { ConfirmDialog } from "./dialogs/ConfirmDialog";
import { NewCardDialog } from "./dialogs/NewCardDialog";
import { NewProjectDialog } from "./dialogs/NewProjectDialog";
import { RemoveProjectDialog } from "./dialogs/RemoveProjectDialog";
import { FilterBar } from "./FilterBar";
import { useGlobalKeys } from "./keyboard";
import { NoticesPanel } from "./notices/NoticesPanel";
import { PhoneNav } from "./PhoneNav";
import { PhoneSheet } from "./PhoneSheet";
import { CommandPalette } from "./palette/CommandPalette";
import { Sidebar } from "./Sidebar";
import { isPhone, isTouch, sizeName } from "./shell-layout";
import { TopBar } from "./TopBar";
import { Toasts } from "./toasts/Toasts";
import { useWindowViewport } from "./use-window-viewport";
import { ViewHeader } from "./ViewHeader";
import { Workspace } from "./Workspace";

/** The whole app: it fills the window, with the sidebar, the main column, and the overlays. */
export function AppRoot() {
  useWindowViewport();
  useGlobalKeys();

  return (
    <div class="fixed inset-0 overflow-hidden bg-canvas">
      <div
        data-app-root="1"
        data-size={sizeName()}
        data-touch={isTouch() ? "1" : "0"}
        class="absolute inset-0 flex bg-canvas text-primary font-sans text-body leading-5 overflow-hidden"
      >
        <Show when={M.S.ready}>
          <Show when={!isPhone()}>
            <Sidebar />
          </Show>
          <div class="flex-1 min-w-0 flex flex-col relative">
            <TopBar />
            <ViewHeader />
            <FilterBar />
            <Workspace />
            <PhoneNav />
          </div>
          <PhoneSheet />
          <NoticesPanel />
          <CommandPalette />
          <ConfirmDialog />
          <NewCardDialog />
          <NewProjectDialog />
          <RemoveProjectDialog />
          <Show when={M.S.onboarding}>
            <div class="absolute inset-0 z-onboarding">
              <Onboarding />
            </div>
          </Show>
          <Show when={M.S.tour && !M.S.onboarding}>
            <div class="absolute inset-0 z-tour pointer-events-none">
              <Tour />
            </div>
          </Show>
          <Toasts />
        </Show>
      </div>
    </div>
  );
}
