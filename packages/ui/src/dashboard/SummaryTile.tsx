import { type JSX, splitProps } from "solid-js";
import { cx } from "../base/cx";

export interface SummaryTileProps
  extends Omit<JSX.ButtonHTMLAttributes<HTMLButtonElement>, "children" | "value"> {
  /** The big number, such as `3` or `$4.20`. */
  value: JSX.Element;
  /** The short label under it, such as `Need you`. */
  label: JSX.Element;
  /** Number color: `default`, `needs-you` (amber), or `danger` (red). */
  tone?: "default" | "needs-you" | "danger";
}

const TONES = {
  default: "text-primary",
  "needs-you": "text-status-needs-you-text",
  danger: "text-status-danger-text",
} as const;

/** One number with a label on the Home summary; it opens a filtered view. */
export function SummaryTile(props: SummaryTileProps) {
  const [local, others] = splitProps(props, ["value", "label", "tone", "class"]);
  return (
    <button
      type="button"
      {...others}
      class={cx(
        "flex flex-col items-start gap-0.5 py-3.5 px-4 rounded-lg border border-border bg-surface text-left hover:border-border-strong",
        local.class,
      )}
    >
      <span
        class={cx("text-tile leading-8.5 font-bold tabular-nums", TONES[local.tone ?? "default"])}
      >
        {local.value}
      </span>
      <span class="text-small leading-4.5 text-secondary">{local.label}</span>
    </button>
  );
}
