import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export type PhoneRowMetaProps = JSX.HTMLAttributes<HTMLSpanElement>;

/**
 * A wrapping line of small facts in a phone list row, 12 px apart across and 2 px between lines.
 * Set the text color with a class.
 */
export function PhoneRowMeta(props: PhoneRowMetaProps) {
  const [local, others] = splitProps(props, ["class", "children"]);
  return (
    <span
      {...others}
      class={cx("flex flex-wrap gap-x-3 gap-y-0.5 text-small leading-4.5", local.class)}
    >
      {local.children}
    </span>
  );
}
