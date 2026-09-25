import { type JSX, Show, splitProps } from "solid-js";
import { cx } from "../base/cx";
import type { LooseString } from "../base/types";
import { StatusIcon } from "../icons/StatusIcon";
import { type StatusKey, statusTone, toneText } from "../icons/status";

export interface StatusLabelProps extends JSX.HTMLAttributes<HTMLSpanElement> {
  state: StatusKey | LooseString;
  /** Icon size in px. Default 14. */
  iconSize?: number;
  /** Show the flag icon. Default true. */
  withIcon?: boolean;
}

const ICON_PX = 14;

/**
 * A card state as colored bold text with its flag: the state line on cards,
 * rows, and tables. The label is the children, such as `Needs you`.
 */
export function StatusLabel(props: StatusLabelProps) {
  const [local, others] = splitProps(props, ["state", "iconSize", "withIcon", "class", "children"]);
  return (
    <span
      {...others}
      class={cx(
        "inline-flex items-center gap-1 font-semibold",
        toneText[statusTone(local.state)],
        local.class,
      )}
    >
      <Show when={local.withIcon ?? true}>
        <StatusIcon state={local.state} size={local.iconSize ?? ICON_PX} />
      </Show>
      {local.children}
    </span>
  );
}
