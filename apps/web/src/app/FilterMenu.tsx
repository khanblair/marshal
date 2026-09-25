import { Icon, MenuItem, MenuLabel } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M } from "~/mock";
import { FilterPopover } from "./FilterPopover";
import { filterGroups } from "./filter-groups";
import { toggleMenu } from "./shell-actions";

const MENU = "filter";

/** The dashed Add filter button and its menu of Status, Role, Agent, Label, and Package values. */
export function FilterMenu() {
  return (
    <div class="relative flex-none">
      <button
        type="button"
        onClick={() => toggleMenu(MENU)}
        aria-expanded={M.S.menu === MENU}
        class="inline-flex items-center gap-1.5 h-8 px-2.5 rounded-sm border border-dashed border-border-strong bg-transparent text-secondary text-small font-medium whitespace-nowrap hover:text-primary hover:bg-surface-hover"
      >
        <Icon name="list-filter" size={14} />
        Add filter
      </button>
      <Show when={M.S.menu === MENU}>
        <FilterPopover side="left">
          <For each={filterGroups()}>
            {(group) => (
              <>
                <MenuLabel>{group.label}</MenuLabel>
                <For each={group.options}>
                  {(option) => (
                    <MenuItem
                      size={30}
                      icon={option.icon}
                      iconClass={option.iconClass}
                      hint={String(option.count)}
                      onClick={() => M.addFilter(option.kind, option.value)}
                    >
                      {option.label}
                    </MenuItem>
                  )}
                </For>
              </>
            )}
          </For>
        </FilterPopover>
      </Show>
    </div>
  );
}
