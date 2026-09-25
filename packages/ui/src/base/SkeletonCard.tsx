import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";
import { Skeleton } from "./Skeleton";
import { SkeletonLines } from "./SkeletonLines";

export type SkeletonCardProps = JSX.HTMLAttributes<HTMLDivElement>;

const TITLE_LINES = 2;
const STATE_WIDTH = "40%";
const ACTIVITY_WIDTH = "65%";
/** The state and activity lines are 13 px text on the real card, so their bars are short. */
const SMALL_LINE_HEIGHT_PX = 10;

/**
 * The shape of a board card while it loads: the card's own box (same padding, border, corners,
 * and 3 px left edge, so nothing shifts when the real card arrives), a two line title, a state
 * line, and an activity line.
 */
export function SkeletonCard(props: SkeletonCardProps) {
  const [local, others] = splitProps(props, ["class"]);
  return (
    <div
      aria-hidden="true"
      {...others}
      class={cx(
        "flex flex-col gap-2 p-3 bg-surface border border-border border-l-3 rounded-md",
        local.class,
      )}
    >
      <SkeletonLines lines={TITLE_LINES} />
      <Skeleton width={STATE_WIDTH} height={SMALL_LINE_HEIGHT_PX} />
      <Skeleton width={ACTIVITY_WIDTH} height={SMALL_LINE_HEIGHT_PX} />
    </div>
  );
}
