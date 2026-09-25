import { Match, Show, Switch } from "solid-js";
import { Tour } from "~/features/tutorial/Tour";
import { M } from "~/mock";
import { Onboarding } from "~/onboarding/Onboarding";
import { AppSkeleton } from "./AppSkeleton";
import {
  ConnectionLostScreen,
  LoadFailedScreen,
  OfflineNotice,
  SignInScreen,
} from "./ConnectionScreens";
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

/** The app itself: the sidebar, the main column, and the overlays. It draws once the store has data. */
function AppFrame() {
  return (
    <>
      <Show when={!isPhone()}>
        <Sidebar />
      </Show>
      <div class="flex-1 min-w-0 flex flex-col relative">
        <Show when={M.S.connection?.state === "reconnecting"}>
          <OfflineNotice />
        </Show>
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
    </>
  );
}

/**
 * The whole app: it fills the window. What it shows follows the link with the daemon: a full screen
 * when the daemon cannot be reached or does not know this device, a skeleton of the app while the
 * first data loads, and the app itself (with a bar at the top while it reconnects) after that.
 * Without a daemon (a test) the link counts as online.
 */
export function AppRoot() {
  useWindowViewport();
  useGlobalKeys();
  const connection = () => M.S.connection?.state ?? "online";

  return (
    <div class="fixed inset-0 overflow-hidden bg-canvas">
      <div
        data-app-root="1"
        data-size={sizeName()}
        data-touch={isTouch() ? "1" : "0"}
        data-connection={connection()}
        class="absolute inset-0 flex bg-canvas text-primary font-sans text-body leading-5 overflow-hidden"
      >
        <Switch>
          <Match when={connection() === "unreachable"}>
            <ConnectionLostScreen />
          </Match>
          <Match when={connection() === "unauthorized"}>
            <SignInScreen />
          </Match>
          <Match when={!M.S.ready && M.S.loadError}>
            {(message) => <LoadFailedScreen message={message()} />}
          </Match>
          <Match when={!M.S.ready}>
            <AppSkeleton />
          </Match>
          <Match when={M.S.ready}>
            <AppFrame />
          </Match>
        </Switch>
      </div>
    </div>
  );
}
