import { For, type JSX, splitProps } from "solid-js";
import { cx } from "./cx";
import { Skeleton } from "./Skeleton";

export interface SkeletonLinesProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /** How many lines of text to stand in for. Default 3. */
  lines?: number;
  /** Height of each line in px. Default 12. */
  lineHeight?: number;
}

const DEFAULT_LINES = 3;
/** Real paragraphs end short, so the last line does too. */
const LAST_LINE_WIDTH = "60%";

/** A paragraph of text while it loads: `lines` bars, the last one shorter. */
export function SkeletonLines(props: SkeletonLinesProps) {
  const [local, others] = splitProps(props, ["lines", "lineHeight", "class"]);
  const rows = () =>
    Array.from({ length: Math.max(1, Math.floor(local.lines ?? DEFAULT_LINES)) }, (_, i) => i);
  return (
    <div aria-hidden="true" {...others} class={cx("flex flex-col gap-2", local.class)}>
      <For each={rows()}>
        {(row) => (
          <Skeleton
            height={local.lineHeight}
            width={row === rows().length - 1 ? LAST_LINE_WIDTH : undefined}
          />
        )}
      </For>
    </div>
  );
}
