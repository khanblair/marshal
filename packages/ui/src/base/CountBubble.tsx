import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export interface CountBubbleProps extends JSX.HTMLAttributes<HTMLSpanElement> {
  /** `ink` (default): notices count. `needs-you`: needs-you count on a solid fill. */
  tone?: "ink" | "needs-you";
}

/**
 * A small round count, 16 px high, usually placed over an icon with absolute
 * position classes, for example `absolute top-0.5 right-0`.
 */
export function CountBubble(props: CountBubbleProps) {
  const [local, others] = splitProps(props, ["tone", "class"]);
  return (
    <span
      {...others}
      class={cx(
        "min-w-4 h-4 px-1 rounded-full text-badge leading-4 font-bold text-center",
        local.tone === "needs-you"
          ? "bg-status-needs-you-solid text-on-status"
          : "bg-ink text-on-ink",
        local.class,
      )}
    />
  );
}
