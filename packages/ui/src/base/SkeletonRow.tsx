import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";
import { Skeleton } from "./Skeleton";

export interface SkeletonRowProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /** The marker before the text: a 28 px avatar (default) or an 8 px dot. */
  leading?: "avatar" | "dot";
}

const AVATAR_PX = 28;
const DOT_PX = 8;
const TITLE_WIDTH = "60%";
const META_WIDTH = "35%";
/** Meta text is smaller than the title, so its bar is too. */
const META_HEIGHT_PX = 10;

/**
 * The shape of a list or project row while it loads: an avatar or a dot, a title bar, and a
 * shorter meta bar under it.
 */
export function SkeletonRow(props: SkeletonRowProps) {
  const [local, others] = splitProps(props, ["leading", "class"]);
  const marker = () => (local.leading === "dot" ? DOT_PX : AVATAR_PX);
  return (
    <div aria-hidden="true" {...others} class={cx("flex items-center gap-3 py-2", local.class)}>
      <Skeleton circle width={marker()} height={marker()} />
      <div class="flex-1 min-w-0 flex flex-col gap-1.5">
        <Skeleton width={TITLE_WIDTH} />
        <Skeleton width={META_WIDTH} height={META_HEIGHT_PX} />
      </div>
    </div>
  );
}
