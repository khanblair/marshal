import { type JSX, splitProps } from "solid-js";
import { cx } from "../base/cx";

export interface DayHeadingProps extends JSX.HTMLAttributes<HTMLHeadingElement> {
  /** Today's heading is in the primary text color; other days are secondary. */
  today?: boolean;
}

/**
 * Heading of one day in a phone day-by-day list, with a line under it. Set the
 * padding with a class: `py-2` in the Timeline list, `pt-2.5 pb-1.5` in the
 * Calendar agenda.
 */
export function DayHeading(props: DayHeadingProps) {
  const [local, others] = splitProps(props, ["today", "class", "children"]);
  return (
    <h3
      {...others}
      class={cx(
        "m-0 px-0 text-body leading-5 font-semibold border-b border-border",
        local.today ? "text-primary" : "text-secondary",
        local.class,
      )}
    >
      {local.children}
    </h3>
  );
}
