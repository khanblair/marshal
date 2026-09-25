import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

/** A width as a share of the container, such as `60%`. */
export type SkeletonPercent = `${number}%`;

export interface SkeletonProps extends Omit<JSX.HTMLAttributes<HTMLSpanElement>, "style"> {
  /** Width in px, or a share of the container such as `60%`. Default: the full width. */
  width?: number | SkeletonPercent;
  /** Height in px. Default 12, a little under the height of one line of body text. */
  height?: number;
  /** A circle, for an avatar or a dot. Corners are 5 px otherwise. */
  circle?: boolean;
  style?: JSX.CSSProperties;
}

const DEFAULT_HEIGHT_PX = 12;

/** A number is px, as every size prop in this package; a percent string is used as it is. */
function cssSize(size: number | SkeletonPercent): string {
  return typeof size === "number" ? `${size}px` : size;
}

/**
 * One rounded block that stands in for a piece of content while it loads. It is decorative
 * (`aria-hidden`) and pulses softly with the same pulse token as the working dot; the base CSS
 * stops that under reduced motion. Put shapes inside a `SkeletonGroup` so a screen reader hears
 * one "Loading" and not one message per block.
 */
export function Skeleton(props: SkeletonProps) {
  const [local, others] = splitProps(props, ["width", "height", "circle", "class", "style"]);
  return (
    <span
      aria-hidden="true"
      {...others}
      class={cx(
        "block flex-none max-w-full bg-border animate-pulse-working",
        local.circle ? "rounded-full" : "rounded-sm",
        local.class,
      )}
      style={{
        ...local.style,
        width: cssSize(local.width ?? "100%"),
        height: cssSize(local.height ?? DEFAULT_HEIGHT_PX),
      }}
    />
  );
}
