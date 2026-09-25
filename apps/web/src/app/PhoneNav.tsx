import { CountBubble, cx, Icon, type IconNameInput } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M, type ViewKey } from "~/mock";
import { isPhone, isProject, isProjectView, totalNeeds } from "./shell-layout";

type NavKey = "home" | "board" | "chat" | "agents" | "more";

interface NavTab {
  key: NavKey;
  label: string;
  icon: IconNameInput;
  go: () => void;
}

const MORE_VIEWS: readonly ViewKey[] = ["list", "timeline", "calendar"];

const view = (key: ViewKey) => () => M.go("project", M.S.route.pid, key);

const TABS: readonly NavTab[] = [
  { key: "home", label: "Home", icon: "house", go: () => M.go("home") },
  { key: "board", label: "Board", icon: "square-kanban", go: view("board") },
  { key: "chat", label: "Chats", icon: "messages-square", go: view("chat") },
  { key: "agents", label: "Agents", icon: "bot", go: view("agents") },
  { key: "more", label: "More", icon: "ellipsis", go: () => M.set({ menu: "more" }) },
];

/** Whether a tab is the current one. More covers Settings and the views it lists. */
function isCurrent(key: NavKey): boolean {
  if (key === "home") return M.S.route.page === "home";
  if (key === "more") {
    return (
      M.S.menu === "more" ||
      M.S.route.page === "settings" ||
      (isProject() && MORE_VIEWS.some(isProjectView))
    );
  }
  return isProjectView(key);
}

/** The phone's bottom navigation: Home, Board, Chats, Agents, and More. */
export function PhoneNav() {
  return (
    <Show when={isPhone()}>
      <nav
        aria-label="Main"
        data-tour="views-phone"
        class="flex-none flex border-t border-border bg-surface pb-[env(safe-area-inset-bottom)] relative z-bar"
      >
        <For each={TABS}>
          {(tab) => (
            <button
              type="button"
              data-tour={tab.key === "more" ? "more-phone" : ""}
              onClick={() => {
                M.set({ openId: null, noticesOpen: false });
                tab.go();
              }}
              aria-current={isCurrent(tab.key) ? "page" : undefined}
              class={cx(
                "flex-1 h-14 flex flex-col items-center justify-center gap-0.5 border-none bg-transparent text-small relative",
                isCurrent(tab.key) ? "text-primary font-semibold" : "text-secondary font-normal",
              )}
            >
              <Show when={isCurrent(tab.key)}>
                <span class="absolute top-0 left-[28%] right-[28%] h-0.5 bg-ink" />
              </Show>
              <Icon name={tab.icon} size={20} />
              <span>{tab.label}</span>
              <Show when={tab.key === "home" && totalNeeds() > 0}>
                <CountBubble
                  tone="needs-you"
                  class="absolute top-1.5 left-[calc(50%+6px)] text-left!"
                >
                  {totalNeeds()}
                </CountBubble>
              </Show>
            </button>
          )}
        </For>
      </nav>
    </Show>
  );
}
