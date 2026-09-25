import { Button, Checkbox } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M } from "~/mock";
import { FilterPopover } from "./FilterPopover";
import { toggleMenu } from "./shell-actions";

const MENU = "cols";

/** The List view's columns, in the design's order. Title is always shown. */
const LIST_COLUMNS: readonly (readonly [key: string, label: string])[] = [
  ["id", "ID"],
  ["title", "Title"],
  ["state", "State"],
  ["role", "Role"],
  ["agent", "Agent"],
  ["model", "Model"],
  ["think", "Thinking mode"],
  ["branch", "Branch"],
  ["ci", "CI"],
  ["cost", "Cost"],
  ["upd", "Updated"],
  ["pkg", "Package"],
];
const REQUIRED_COLUMN = "title";

/** Flips a column. Title stays on, so its box goes back to how the store has it. */
export function toggleColumn(key: string, box: HTMLInputElement): void {
  if (key === REQUIRED_COLUMN) {
    box.checked = !!M.S.listCols[key];
    return;
  }
  M.S.listCols[key] = !M.S.listCols[key];
}

/** The Columns button and its list of checkboxes, for the List view. */
export function ColumnsMenu() {
  return (
    <div class="relative flex-none">
      <Button
        icon="columns-3"
        iconSize={14}
        class="px-2.5! text-small"
        onClick={() => toggleMenu(MENU)}
        aria-expanded={M.S.menu === MENU}
      >
        Columns
      </Button>
      <Show when={M.S.menu === MENU}>
        <FilterPopover side="right">
          <For each={LIST_COLUMNS}>
            {([key, label]) => (
              <label class="flex items-center gap-2 min-h-8 px-2 rounded-sm text-small cursor-pointer hover:bg-surface-hover">
                <Checkbox
                  size={16}
                  checked={!!M.S.listCols[key]}
                  onChange={(event) => toggleColumn(key, event.currentTarget)}
                />
                {label}
              </label>
            )}
          </For>
        </FilterPopover>
      </Show>
    </div>
  );
}
