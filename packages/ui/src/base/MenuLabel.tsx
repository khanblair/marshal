import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export type MenuLabelProps = JSX.HTMLAttributes<HTMLDivElement>;

/** A small group heading inside a `Menu`, such as Status or People. */
export function MenuLabel(props: MenuLabelProps) {
  const [local, others] = splitProps(props, ["class"]);
  return (
    <div
      {...others}
      class={cx("px-2 pt-2 pb-1 text-caption leading-4 text-secondary font-semibold", local.class)}
    />
  );
}
