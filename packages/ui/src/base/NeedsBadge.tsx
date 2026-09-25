import { type JSX, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import { cx } from "./cx";

export interface NeedsBadgeProps extends Omit<JSX.HTMLAttributes<HTMLSpanElement>, "children"> {
  /** How many cards need you. */
  count: number;
  /** 20 (default, 12 px text) or 22 (13 px text, phone sheets). */
  size?: 20 | 22;
}

const ICON_PX = 12;
const LARGE = 22;

/**
 * The "needs you" count: a raised hand and a number on the needs-you tint.
 * Used in the sidebar, the phone header, and the phone project picker.
 */
export function NeedsBadge(props: NeedsBadgeProps) {
  const [local, others] = splitProps(props, ["count", "size", "class"]);
  return (
    <span
      {...others}
      class={cx(
        "inline-flex items-center gap-0.75 px-1.5 rounded-xs bg-status-needs-you-subtle text-status-needs-you-text font-semibold",
        local.size === LARGE ? "h-5.5 text-small" : "h-5 text-caption",
        local.class,
      )}
    >
      <Icon name="st-needs" size={ICON_PX} />
      {local.count}
    </span>
  );
}
