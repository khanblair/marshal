import { cx, Icon, Input, Kbd, Select, Tag } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M, type SwimKey } from "~/mock";
import { ColumnsMenu } from "./ColumnsMenu";
import { FilterMenu } from "./FilterMenu";
import { chipValue, FILTER_KEY_LABEL } from "./filter-groups";
import { SavedViewsMenu } from "./SavedViewsMenu";
import { isDesktop, isPhone, isProject } from "./shell-layout";

const FILTER_BAR_VIEWS = ["board", "list", "timeline"];
const SWIMLANES: readonly { value: SwimKey; label: string }[] = [
  { value: "none", label: "None" },
  { value: "role", label: "Role" },
  { value: "agent", label: "Agent" },
  { value: "package", label: "Package" },
  { value: "label", label: "Label" },
];

const pid = (): string => M.S.route.pid ?? "";
const filters = () => M.S.filters[pid()] ?? [];
const view = () => M.S.route.view;

/** Board, List, and Timeline show the bar; on phones the open card covers it. */
const showFilterBar = (): boolean =>
  isProject() && FILTER_BAR_VIEWS.includes(view()) && !(isPhone() && M.S.openId);

const hasFilters = (): boolean => filters().length > 0 || !!M.S.query[pid()];

function setSwimlane(value: string): void {
  const swim = SWIMLANES.find((lane) => lane.value === value);
  if (!swim) return;
  M.S.swim[pid()] = swim.value;
  M.S.savedView[pid()] = null;
}

function SearchField() {
  return (
    <label class="relative flex items-center flex-none">
      <span class="absolute left-2 inline-flex text-muted">
        <Icon name="search" size={14} />
      </span>
      <Input
        data-search="1"
        value={M.S.query[pid()] || ""}
        onInput={(event) => {
          M.S.query[pid()] = event.currentTarget.value;
        }}
        placeholder="Filter cards"
        aria-label="Filter cards"
        class={cx("px-7! text-small", isPhone() ? "w-[150px]" : "w-[200px]")}
      />
      <Show when={isDesktop()}>
        <Kbd tone="muted" class="absolute right-1.5">
          /
        </Kbd>
      </Show>
    </label>
  );
}

function SwimlaneSelect() {
  return (
    <label class="flex-none flex items-center gap-1.5 text-small text-secondary">
      <span>Swimlanes</span>
      <Select
        class="px-1.5! text-small"
        options={SWIMLANES}
        value={M.S.swim[pid()]}
        onChange={(event) => setSwimlane(event.currentTarget.value)}
      />
    </label>
  );
}

/**
 * The search field, filter chips, Add filter and saved views menus, and the Swimlanes
 * or Columns control, under the view tabs of Board, List, and Timeline.
 */
export function FilterBar() {
  return (
    <Show when={showFilterBar()}>
      <div
        class={cx(
          "flex-none flex items-center gap-2 py-2 border-b border-border bg-surface relative z-[9]",
          isPhone() ? "flex-nowrap overflow-x-auto px-2" : "flex-wrap overflow-x-visible px-4",
        )}
      >
        <SearchField />
        <For each={filters()}>
          {(filter) => (
            <Tag
              removeLabel={`Remove filter ${FILTER_KEY_LABEL[filter.k]} ${filter.v}`}
              onRemove={() => M.removeFilter(filter.k, filter.v)}
            >
              <span class="text-secondary">{FILTER_KEY_LABEL[filter.k]}</span>
              <span class="font-semibold">{chipValue(filter.k, filter.v)}</span>
            </Tag>
          )}
        </For>
        <FilterMenu />
        <SavedViewsMenu />
        <Show when={hasFilters()}>
          <button
            type="button"
            onClick={() => M.clearFilters()}
            class="flex-none h-8 px-2 border-none rounded-sm bg-transparent text-secondary text-small whitespace-nowrap hover:bg-surface-hover"
          >
            Clear filters
          </button>
        </Show>
        <div class="flex-1" />
        <Show when={view() === "board" && !isPhone()}>
          <SwimlaneSelect />
        </Show>
        <Show when={view() === "list" && !isPhone()}>
          <ColumnsMenu />
        </Show>
      </div>
    </Show>
  );
}
