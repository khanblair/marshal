import { Button, Icon, Input, MenuItem, MenuSeparator } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M, type SavedView } from "~/mock";
import { FilterPopover } from "./FilterPopover";
import { toggleMenu } from "./shell-actions";

const MENU = "views";
const UNSAVED_LABEL = "Unsaved view";

const pid = (): string => M.S.route.pid ?? "";

/** The short description after a saved view's name: its swimlanes, or its filter count. */
export function viewDescription(view: SavedView): string {
  if (view.swim !== "none") return `By ${view.swim}`;
  if (!view.f.length) return "";
  return `${view.f.length} ${view.f.length > 1 ? "filters" : "filter"}`;
}

function saveFromForm(event: SubmitEvent & { currentTarget: HTMLFormElement }): void {
  event.preventDefault();
  const name = new FormData(event.currentTarget).get("viewname");
  M.saveView(typeof name === "string" ? name.trim() : "");
}

/** The saved views button, its list of views, and the field that saves the current filters. */
export function SavedViewsMenu() {
  return (
    <div class="relative flex-none">
      <Button
        icon="bookmark"
        iconSize={14}
        class="px-2.5! text-small whitespace-nowrap"
        onClick={() => toggleMenu(MENU)}
        aria-expanded={M.S.menu === MENU}
      >
        <span>{M.S.savedView[pid()] || UNSAVED_LABEL}</span>
        <Icon name="chevron-down" size={14} />
      </Button>
      <Show when={M.S.menu === MENU}>
        <FilterPopover side="left">
          <For each={M.S.savedViews[pid()] ?? []}>
            {(view) => (
              <MenuItem
                kind="radio"
                size={30}
                checked={view.name === M.S.savedView[pid()]}
                hint={viewDescription(view)}
                onClick={() => M.applyView(view.name)}
              >
                {view.name}
              </MenuItem>
            )}
          </For>
          <MenuSeparator />
          <form onSubmit={saveFromForm} class="flex gap-1.5 p-0.5">
            <Input
              name="viewname"
              placeholder="Name this view"
              aria-label="Name this view"
              class="flex-1 min-w-0 h-7.5! px-2! text-small"
            />
            <button
              type="submit"
              class="h-7.5 px-2.5 rounded-sm border border-border-strong bg-surface text-small font-medium"
            >
              Save view
            </button>
          </form>
        </FilterPopover>
      </Show>
    </div>
  );
}
