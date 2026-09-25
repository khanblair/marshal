import { type JSX, Show, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import type { IconNameInput } from "../icons/icon-names";
import { cx } from "./cx";

/** Row height in px. 30 and 36 are minimum heights; 34 is the member picker; 48 is a phone sheet row. */
export type MenuItemSize = 30 | 32 | 34 | 36 | 48;

export interface MenuItemProps extends JSX.ButtonHTMLAttributes<HTMLButtonElement> {
  icon?: IconNameInput;
  /** Icon size in px. Default 14, or 16 at 36 and 18 at 48. */
  iconSize?: number;
  /** Classes for the icon, for example a status color. */
  iconClass?: string;
  /** Leading content instead of an icon, such as an `Avatar`. */
  leading?: JSX.Element;
  /** Red text and a red hover, for delete and remove. */
  danger?: boolean;
  /** Muted trailing text, such as a count or a description. */
  hint?: JSX.Element;
  /** 30, 32 (default), 34, 36, or 48. */
  size?: MenuItemSize;
  /**
   * `item` (default): `menuitem`. `radio`: `menuitemradio` with a check slot
   * in front. `check`: `menuitemcheckbox` with a check at the end. `plain`: a
   * button with no menu role (phone sheet rows).
   */
  kind?: MenuItemKind;
  /** Checked state for `radio` and `check`. */
  checked?: boolean;
  /** Marks the current page (`aria-current`) with the selected fill. */
  current?: boolean;
}

const SIZES: Record<MenuItemSize, string> = {
  30: "min-h-7.5 gap-2 px-2 rounded-sm text-small",
  32: "h-8 gap-2 px-2 rounded-sm text-small",
  34: "h-8.5 gap-2 px-2 rounded-sm text-small",
  36: "min-h-9 gap-2.5 px-2.5 rounded-sm",
  48: "min-h-12 gap-3 px-2 rounded-md text-lead",
};

const ICON_PX: Record<MenuItemSize, number> = { 30: 14, 32: 14, 34: 14, 36: 16, 48: 18 };
export type MenuItemKind = "item" | "radio" | "check" | "plain";
const ROLES: Record<MenuItemKind, "menuitem" | "menuitemradio" | "menuitemcheckbox" | undefined> = {
  item: "menuitem",
  radio: "menuitemradio",
  check: "menuitemcheckbox",
  plain: undefined,
};
const DEFAULT_SIZE: MenuItemSize = 32;
const CHECK_PX = 14;

function colorClass(danger: boolean, current: boolean): string {
  if (danger) return "bg-transparent text-status-danger-text hover:bg-status-danger-subtle";
  return cx(
    "text-primary hover:bg-surface-hover",
    current ? "bg-surface-selected" : "bg-transparent",
  );
}

/** One row in a `Menu`: icon and label, with optional hint and check. */
export function MenuItem(props: MenuItemProps) {
  const [local, others] = splitProps(props, [
    "icon",
    "iconSize",
    "iconClass",
    "leading",
    "danger",
    "hint",
    "size",
    "kind",
    "checked",
    "current",
    "class",
    "children",
  ]);
  const size = () => local.size ?? DEFAULT_SIZE;
  const kind = () => local.kind ?? "item";
  const checkable = () => kind() === "radio" || kind() === "check";
  return (
    // biome-ignore lint/a11y/useAriaPropsSupportedByRole: aria-checked is only set when the role is menuitemradio or menuitemcheckbox
    <button
      type="button"
      role={ROLES[kind()]}
      aria-checked={checkable() ? !!local.checked : undefined}
      aria-current={local.current ? "page" : undefined}
      {...others}
      class={cx(
        "w-full flex items-center border-none text-left",
        SIZES[size()],
        colorClass(!!local.danger, !!local.current),
        local.class,
      )}
    >
      <Show when={kind() === "radio"}>
        <span class="w-3.5 inline-flex">
          <Show when={local.checked}>
            <Icon name="check" size={CHECK_PX} />
          </Show>
        </span>
      </Show>
      {local.leading}
      <Show when={local.icon}>
        {(name) => (
          <Icon name={name()} size={local.iconSize ?? ICON_PX[size()]} class={local.iconClass} />
        )}
      </Show>
      <span class="flex-1">{local.children}</span>
      <Show when={local.hint}>
        <span class="text-caption text-muted">{local.hint}</span>
      </Show>
      <Show when={kind() === "check" && local.checked}>
        <Icon name="check" size={CHECK_PX} />
      </Show>
    </button>
  );
}
