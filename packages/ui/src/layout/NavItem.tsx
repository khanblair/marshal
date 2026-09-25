import { type JSX, Show, splitProps } from "solid-js";
import { cx } from "../base/cx";
import { Icon } from "../icons/Icon";
import type { IconNameInput } from "../icons/icon-names";

export interface NavItemProps
  extends Omit<JSX.ButtonHTMLAttributes<HTMLButtonElement>, "children"> {
  icon: IconNameInput;
  /** Visible label, and the default `title`. Hidden when `collapsed`. */
  label: string;
  /** The current page: selected fill, an ink bar on the left, `aria-current="page"`. */
  current?: boolean;
  /** Icon-only, centered (collapsed sidebar). */
  collapsed?: boolean;
  /** Content after the label, such as a `NeedsBadge`. */
  trailing?: JSX.Element;
}

const ICON_PX = 16;

/** A main navigation row in the sidebar: Home and Settings. */
export function NavItem(props: NavItemProps) {
  const [local, others] = splitProps(props, [
    "icon",
    "label",
    "current",
    "collapsed",
    "trailing",
    "class",
  ]);
  return (
    <button
      type="button"
      title={local.label}
      aria-current={local.current ? "page" : undefined}
      {...others}
      class={cx(
        "relative flex items-center gap-2 min-h-8 border-none rounded-sm text-primary font-medium text-left hover:bg-surface-hover",
        local.collapsed ? "px-0 justify-center" : "px-2 justify-start",
        local.current ? "bg-surface-selected" : "bg-transparent",
        local.class,
      )}
    >
      <Show when={local.current}>
        <span class="absolute -left-2 top-1.5 bottom-1.5 w-0.5 rounded-2xs bg-ink" />
      </Show>
      <Icon name={local.icon} size={ICON_PX} />
      <Show when={!local.collapsed}>
        <span class="flex-1">{local.label}</span>
      </Show>
      {local.trailing}
    </button>
  );
}
