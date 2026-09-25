import { type JSX, onCleanup, onMount, Show, splitProps } from "solid-js";
import { Scrim } from "../layout/Scrim";
import { cx } from "./cx";

export interface MenuProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /**
   * Called on Escape and on a pointer press outside the menu and its trigger.
   * Without it, the menu does not close by itself.
   */
  onClose?: () => void;
  /** The button that opens the menu. Presses on it do not count as outside. */
  trigger?: () => Element | undefined;
  /**
   * Show as a bottom sheet over a scrim (phone). Otherwise it is a popover:
   * place it with classes such as `absolute right-0 top-[calc(100%+2px)] w-[200px] z-[170]`.
   */
  sheet?: boolean;
  /**
   * Dim the page behind a sheet. Default true. The avatar menu's phone sheet sets
   * it false: it sits over a clear click catcher instead.
   */
  scrim?: boolean;
}

const ITEM_SELECTOR = '[role^="menuitem"]:not([disabled])';

/** Moves focus between items with the arrow keys, Home, and End. */
function focusItem(menu: HTMLElement, key: string) {
  const items = Array.from(menu.querySelectorAll<HTMLElement>(ITEM_SELECTOR));
  if (!items.length) return false;
  const current = items.indexOf(document.activeElement as HTMLElement);
  const last = items.length - 1;
  const targets: Record<string, number> = {
    ArrowDown: current < 0 || current === last ? 0 : current + 1,
    ArrowUp: current <= 0 ? last : current - 1,
    Home: 0,
    End: last,
  };
  const target = targets[key];
  if (target === undefined) return false;
  items[target]?.focus();
  return true;
}

/**
 * The floating menu panel (`role="menu"`) with `MenuItem` rows. As a popover it
 * is raised with a border and shadow; with `sheet` it becomes a phone bottom
 * sheet over a scrim, as the design does on phones.
 */
export function Menu(props: MenuProps) {
  const [local, others] = splitProps(props, [
    "onClose",
    "trigger",
    "sheet",
    "scrim",
    "class",
    "onKeyDown",
  ]);
  let panel: HTMLDivElement | undefined;

  const onPointerDown = (event: PointerEvent) => {
    const target = event.target as Node | null;
    if (!target || panel?.contains(target) || local.trigger?.()?.contains(target)) return;
    local.onClose?.();
  };
  onMount(() => document.addEventListener("pointerdown", onPointerDown, true));
  onCleanup(() => document.removeEventListener("pointerdown", onPointerDown, true));

  const onKeyDown: JSX.EventHandler<HTMLDivElement, KeyboardEvent> = (event) => {
    if (typeof local.onKeyDown === "function") local.onKeyDown(event);
    if (event.key === "Escape" && local.onClose) {
      event.preventDefault();
      event.stopPropagation();
      local.onClose();
    } else if (panel && focusItem(panel, event.key)) {
      event.preventDefault();
    }
  };

  return (
    <>
      <Show when={local.sheet && local.scrim !== false}>
        <Scrim tone="sheet" fixed class="z-[290]" />
      </Show>
      <div
        ref={panel}
        role="menu"
        {...others}
        onKeyDown={onKeyDown}
        class={cx(
          local.sheet
            ? "fixed left-0 right-0 bottom-0 z-sheet max-h-[75%] overflow-auto pt-2 px-2 pb-[calc(12px+env(safe-area-inset-bottom))] rounded-t-xl bg-surface-raised shadow-e2"
            : "p-1.5 rounded-lg border border-border bg-surface-raised shadow-e1",
          local.class,
        )}
      />
    </>
  );
}
