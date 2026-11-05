import { cx, Icon, IconButton, NavItem, NeedsBadge, Scrim } from "@marshal/ui";
import { Show } from "solid-js";
import { openUpdates } from "~/features/updates/UpdateNotice";
import { updates } from "~/features/updates/updates";
import { M } from "~/mock";
import marshalMark from "../../../mobile/splash/marshal-mark.png";
import { ProjectList } from "./ProjectList";
import { isDesktop, sidebarOpen, sideOverlay, totalNeeds } from "./shell-layout";
import { closeSide, goHome, goSettings, toggleSidebar } from "./sidebar-actions";

/** The left navigation: Home, the projects, and Settings. Icon-only when collapsed. */
export function Sidebar() {
  const wide = () => isDesktop() && !M.S.sidebarCollapsed;
  return (
    <div
      class={cx(
        "relative flex-none transition-[width] duration-base ease-standard",
        wide() ? "w-sidebar" : "w-sidebar-collapsed",
      )}
    >
      <Show when={sideOverlay()}>
        <Scrim tone="side" class="z-[155] w-screen" onClick={closeSide} />
      </Show>
      <nav
        aria-label="Main"
        class={cx(
          "absolute flex flex-col border-r border-border bg-canvas",
          sideOverlay() ? "left-0 top-0 bottom-0 w-sidebar z-side shadow-e2" : "inset-0",
        )}
      >
        <div class="h-[calc(48px+var(--safe-top))] pt-[var(--safe-top)] flex-none flex items-center gap-2 pl-4 pr-2 border-b border-border">
          <img
            src={marshalMark}
            alt=""
            width={20}
            height={20}
            class="marshal-mark size-5 shrink-0 object-contain"
          />
          <Show when={sidebarOpen()}>
            <span class="font-bold text-subtitle leading-5.5 flex-1">Marshal</span>
            <IconButton
              label="Collapse sidebar"
              title="Collapse sidebar"
              icon="panel-left-close"
              onClick={toggleSidebar}
            />
          </Show>
        </div>
        <div class="pt-3 px-2 pb-1 flex flex-col gap-0.5">
          <Show when={!sidebarOpen()}>
            <button
              type="button"
              onClick={toggleSidebar}
              aria-label="Expand sidebar"
              title="Expand sidebar"
              class="flex items-center justify-center h-10 border-none rounded-sm bg-transparent text-secondary hover:bg-surface-hover"
            >
              <Icon name="panel-left-open" size={16} />
            </button>
          </Show>
          <NavItem
            icon="house"
            label="Home"
            current={M.S.route.page === "home"}
            collapsed={!sidebarOpen()}
            onClick={goHome}
            trailing={
              <Show when={totalNeeds() > 0 && sidebarOpen()}>
                <NeedsBadge
                  count={totalNeeds()}
                  title={`${totalNeeds()} cards need you across all projects`}
                />
              </Show>
            }
          />
        </div>
        <ProjectList />
        <div class="p-2 border-t border-border">
          <NavItem
            icon="download"
            label="Updates"
            aria-label={
              updates.state.status === "installed"
                ? "Updates ready to restart"
                : updates.state.status === "available"
                  ? "Updates available"
                  : "Updates"
            }
            current={M.S.route.page === "settings" && M.S.settingsSection === "updates"}
            collapsed={!sidebarOpen()}
            class="w-full"
            onClick={openUpdates}
            trailing={
              <Show when={["available", "installed"].includes(updates.state.status)}>
                <span class="size-2 shrink-0 rounded-full bg-ink" aria-hidden="true" />
              </Show>
            }
          />
          <NavItem
            icon="settings"
            label="Settings"
            current={M.S.route.page === "settings" && M.S.settingsSection !== "updates"}
            collapsed={!sidebarOpen()}
            class="w-full"
            onClick={goSettings}
          />
        </div>
      </nav>
    </div>
  );
}
