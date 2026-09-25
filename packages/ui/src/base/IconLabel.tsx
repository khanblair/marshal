import { type JSX, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import type { IconNameInput } from "../icons/icon-names";
import { cx } from "./cx";

export interface IconLabelProps extends JSX.HTMLAttributes<HTMLSpanElement> {
  icon: IconNameInput;
  /** Icon size in px. Default 14. */
  size?: number;
  /** Space between icon and text in px: 3, 4 (default), or 6. */
  gap?: 3 | 4 | 6;
  /** Classes for the icon only, such as a status color. */
  iconClass?: string;
}

const GAPS = { 3: "gap-0.75", 4: "gap-1", 6: "gap-1.5" } as const;
const DEFAULT_ICON_PX = 14;
const DEFAULT_GAP = 4;

/**
 * An inline icon followed by text: card meta such as the branch, cost,
 * comment count, Pinned, or Asleep. The text takes the color of the span.
 */
export function IconLabel(props: IconLabelProps) {
  const [local, others] = splitProps(props, [
    "icon",
    "size",
    "gap",
    "iconClass",
    "class",
    "children",
  ]);
  return (
    <span
      {...others}
      class={cx("inline-flex items-center", GAPS[local.gap ?? DEFAULT_GAP], local.class)}
    >
      <Icon name={local.icon} size={local.size ?? DEFAULT_ICON_PX} class={local.iconClass} />
      {local.children}
    </span>
  );
}
