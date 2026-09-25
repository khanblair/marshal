import { splitProps } from "solid-js";
import { cx } from "../base/cx";
import type { LooseString } from "../base/types";
import { Icon, type IconProps } from "./Icon";
import { type StatusKey, statusIcon, statusTone, toneIconText, toneSolidText } from "./status";

export interface StatusIconProps extends Omit<IconProps, "name"> {
  /** Card state. Unknown states draw the backlog circle. */
  state: StatusKey | LooseString;
  /**
   * `card` (default) colors backlog muted, as on cards, lists, and rows.
   * `column` colors backlog with the strong border color, as in column headers,
   * column tabs, filter menus, and search results.
   */
  palette?: "card" | "column";
}

/** The state's flag icon in the state's solid color. */
export function StatusIcon(props: StatusIconProps) {
  const [local, others] = splitProps(props, ["state", "palette", "class"]);
  const color = () => {
    const tone = statusTone(local.state);
    return local.palette === "column" ? toneSolidText[tone] : toneIconText[tone];
  };
  return <Icon {...others} name={statusIcon(local.state)} class={cx(color(), local.class)} />;
}
