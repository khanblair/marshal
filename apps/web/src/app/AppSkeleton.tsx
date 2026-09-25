import { Skeleton, SkeletonCard, SkeletonGroup, SkeletonRow } from "@marshal/ui";
import { For, Show } from "solid-js";
import { isDesktop, isPhone } from "./shell-layout";

const SIDEBAR_ROWS = 4;
const COLUMN_COUNT = 4;
// biome-ignore lint/style/noMagicNumbers: the shape of the placeholder columns, not a quantity
const CARDS_PER_COLUMN = [3, 2, 2, 1] as const;
const TITLE_WIDTH_PX = 120;
const LOGO_PX = 20;
const AVATAR_PX = 28;

/** The sidebar's own width and header, with a row of placeholders for each project. */
function SidebarSkeleton() {
  return (
    <div
      class={`flex-none flex flex-col border-r border-border ${isDesktop() ? "w-sidebar" : "w-sidebar-collapsed"}`}
    >
      <div class="h-12 flex-none flex items-center gap-2 pl-4 pr-2 border-b border-border">
        <Skeleton circle width={LOGO_PX} height={LOGO_PX} />
      </div>
      <div class="flex flex-col gap-0.5 p-2">
        <For each={Array.from({ length: SIDEBAR_ROWS })}>
          {() =>
            isDesktop() ? (
              <SkeletonRow />
            ) : (
              <Skeleton circle width={AVATAR_PX} height={AVATAR_PX} class="mx-auto my-2" />
            )
          }
        </For>
      </div>
    </div>
  );
}

/** The board's columns as placeholders: one column on a phone, four elsewhere. */
function BoardSkeleton() {
  const columns = () => (isPhone() ? CARDS_PER_COLUMN.slice(0, 1) : CARDS_PER_COLUMN);
  return (
    <div class="flex-1 min-h-0 flex gap-3 p-3 bg-surface-sunken">
      <For each={columns().slice(0, COLUMN_COUNT)}>
        {(cards) => (
          <div class="flex-1 min-w-0 flex flex-col gap-2">
            <For each={Array.from({ length: cards })}>{() => <SkeletonCard />}</For>
          </div>
        )}
      </For>
    </div>
  );
}

/**
 * What the app shows before it has any data, drawn in the app's own layout (the sidebar, the top
 * bar, and the board's columns), so nothing jumps when the real screens arrive. It is one status
 * for a screen reader: "Loading Marshal".
 */
export function AppSkeleton() {
  return (
    <SkeletonGroup label="Loading Marshal" aria-busy="true" class="flex flex-1 min-w-0 min-h-0">
      <Show when={!isPhone()}>
        <SidebarSkeleton />
      </Show>
      <div class="flex-1 min-w-0 flex flex-col">
        <div class="h-12 flex-none flex items-center px-4 border-b border-border">
          <Skeleton width={TITLE_WIDTH_PX} />
        </div>
        <BoardSkeleton />
      </div>
    </SkeletonGroup>
  );
}
