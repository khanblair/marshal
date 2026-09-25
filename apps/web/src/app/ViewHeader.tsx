import { Button, SegmentedControl } from "@marshal/ui";
import { Show } from "solid-js";
import { M, type ViewKey } from "~/mock";
import { isDesktop, isPhone, isProject, isTouch, maxPanes, modKey } from "./shell-layout";

/** Tab heights in px: touch screens get larger tabs. */
const TAB_TOUCH_PX = 40;
const TAB_PX = 26;
/** The views whose header offers New card. */
const NEW_CARD_VIEWS: readonly ViewKey[] = ["board", "list", "timeline", "agents"];

/** Opens one more pane, showing the first view that is not on screen yet. */
function addSplit(): void {
  const used: ViewKey[] = [M.S.route.view, ...M.S.split];
  const next = M.VIEWS.find((v) => !used.includes(v.key)) ?? M.VIEWS[0];
  if (next) M.S.split.push(next.key);
}

const canSplit = (): boolean => isProject() && M.S.split.length < maxPanes() && !M.S.detailExpanded;
const showNewCard = (): boolean => isProject() && NEW_CARD_VIEWS.includes(M.S.route.view);

/**
 * The project's view tabs with Split view and New card. Hidden on phones, which use
 * the bottom navigation. Port of the design's view header row.
 */
export function ViewHeader() {
  const options = M.VIEWS.map((v, i) => ({
    value: v.key,
    label: v.label,
    icon: v.icon,
    title: `${v.label} (${modKey()} ${i + 1})`,
  }));
  return (
    <Show when={isProject() && !isPhone()}>
      <div class="flex-none flex items-center gap-2 py-1.5 px-4 border-b border-border bg-surface relative z-[11]">
        <SegmentedControl
          kind="tabs"
          label="Views"
          data-tour="views"
          size={isTouch() ? TAB_TOUCH_PX : TAB_PX}
          compact
          class="overflow-x-auto min-w-0"
          options={options}
          value={M.S.route.view}
          onValueChange={(view) => M.setView(view)}
        />
        <div class="flex-1" />
        <Show when={canSplit()}>
          <Button
            icon="columns-2"
            iconSize={14}
            class="px-2.5! text-small"
            title="Open another view beside this one"
            onClick={addSplit}
          >
            Split view
          </Button>
        </Show>
        <Show when={showNewCard()}>
          <Button
            variant="primary"
            icon="plus"
            kbd={isDesktop() ? "N" : undefined}
            onClick={() => M.newCard()}
          >
            <span>New card</span>
          </Button>
        </Show>
      </div>
    </Show>
  );
}
