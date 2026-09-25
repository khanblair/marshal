import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export type MenuSeparatorProps = JSX.HTMLAttributes<HTMLHRElement>;

/** A 1 px line between groups in a `Menu`. */
export function MenuSeparator(props: MenuSeparatorProps) {
  const [local, others] = splitProps(props, ["class"]);
  return <hr {...others} class={cx("h-px my-1.5 mx-0 border-none bg-border", local.class)} />;
}
