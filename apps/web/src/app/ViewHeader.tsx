import { Badge, Button, cx, SegmentedControl } from "@marshal/ui";
import { Show } from "solid-js";
import { M, type ViewKey } from "~/mock";
import { isDesktop, isPhone, isProject, isTouch, maxPanes, modKey } from "./shell-layout";

/** Tab heights in px: touch screens get larger tabs. */
const TAB_TOUCH_PX = 40;
const TAB_PX = 26;
/** The views whose header offers New card. */
const NEW_CARD_VIEWS: readonly ViewKey[] = ["board", "list", "timeline", "agents"];

/**
 * The view tabs need 500 px, and Split view and New card need 220 px more with their gaps.
 * When the header has less than 720 px inside its padding (a small tablet, or a wide phone),
 * both buttons keep only their icon and the label stays for screen readers, so the tabs keep
 * their room and the buttons never wrap. Sized by the header itself, not by the window.
 */
const LABEL_HIDDEN_NARROW = "@max-[720px]:sr-only";
const ICON_ONLY_NARROW = "@max-[720px]:px-3!";

/** Opens one more pane, showing the first view that is not on screen yet. */
function addSplit(): void {
  const used: ViewKey[] = [M.S.route.view, ...M.S.split];
  const next = M.VIEWS.find((v) => !used.includes(v.key)) ?? M.VIEWS[0];
  if (next) M.S.split.push(next.key);
}

const canSplit = (): boolean => isProject() && M.S.split.length < maxPanes() && !M.S.detailExpanded;
const showNewCard = (): boolean => isProject() && NEW_CARD_VIEWS.includes(M.S.route.view);

/** Where finished cards land. Nothing is drawn for a project with no branch known. */
function MergeTarget() {
  const branch = () => M.proj(M.S.route.pid)?.integrationBranch;
  return (
    <Show when={branch()}>
      {(name) => (
        <Badge
          tone="outline"
          size={22}
          icon="git-merge"
          title={`Finished cards merge into ${name()}`}
          class="min-w-0 max-w-60 @max-[720px]:max-w-32 font-normal!"
        >
          <span class="truncate">
            Merging into <span class="font-mono">{name()}</span>
          </span>
        </Badge>
      )}
    </Show>
  );
}

/**
 * The project's view tabs with the merge target, Split view, and New card. Hidden on phones, which use
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
      <div class="@container flex-none flex items-center gap-2 py-1.5 px-4 border-b border-border bg-surface relative z-[11]">
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
        <MergeTarget />
        <Show when={canSplit()}>
          <Button
            icon="columns-2"
            iconSize={14}
            class={cx("px-2.5! text-small", ICON_ONLY_NARROW)}
            title="Open another view beside this one"
            onClick={addSplit}
          >
            <span class={LABEL_HIDDEN_NARROW}>Split view</span>
          </Button>
        </Show>
        <Show when={showNewCard()}>
          <Button
            variant="primary"
            icon="plus"
            kbd={isDesktop() ? "N" : undefined}
            class={ICON_ONLY_NARROW}
            title="New card"
            onClick={() => M.newCard()}
          >
            <span class={LABEL_HIDDEN_NARROW}>New card</span>
          </Button>
        </Show>
      </div>
    </Show>
  );
}
