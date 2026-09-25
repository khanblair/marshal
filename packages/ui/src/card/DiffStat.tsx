import { type JSX, splitProps } from "solid-js";
import { cx } from "../base/cx";

export interface DiffStatProps extends Omit<JSX.HTMLAttributes<HTMLSpanElement>, "children"> {
  /** Added lines, shown after a plus sign. Pass a formatted string for totals, such as `1,304`. */
  added: number | string;
  /** Removed lines, shown after a minus sign (U+2212). */
  removed: number | string;
}

/**
 * Added and removed line counts in the diff colors, in 12 px monospace:
 * the diff summary, each changed file, and the diff row in a chat.
 */
export function DiffStat(props: DiffStatProps) {
  const [local, others] = splitProps(props, ["added", "removed", "class"]);
  return (
    <span
      {...others}
      class={cx("inline-flex items-center gap-2.5 font-mono text-caption", local.class)}
    >
      <span class="text-diff-added-text">+{local.added}</span>
      <span class="text-diff-removed-text">
        {"−"}
        {local.removed}
      </span>
    </span>
  );
}
