import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export type ProgressTone = "ready" | "neutral" | "working" | "needs-you";

export interface ProgressBarProps extends Omit<JSX.HTMLAttributes<HTMLDivElement>, "children"> {
  /** Percent, 0 to 100. */
  value: number;
  /** Accessible name, such as `Merge progress`. */
  label: string;
  /**
   * Fill color. `ready`: teal on a teal tint (merge). `neutral` (default):
   * secondary gray. `working`: green (a finished checklist). `needs-you`:
   * amber (context near full).
   */
  tone?: ProgressTone;
  /** Height in px: 4 or 6 (default). */
  size?: 4 | 6;
  /** A `meter` with a light border (context window), instead of a `progressbar`. */
  meter?: boolean;
}

const FILLS: Record<ProgressTone, string> = {
  ready: "bg-status-ready-solid",
  neutral: "bg-text-secondary",
  working: "bg-status-working-solid",
  "needs-you": "bg-status-needs-you-solid",
};

const MAX = 100;
const SMALL = 4;

/** A thin rounded bar. Set its width with a class, such as `flex-1` or `w-14`. */
export function ProgressBar(props: ProgressBarProps) {
  const [local, others] = splitProps(props, ["value", "label", "tone", "size", "meter", "class"]);
  const tone = () => local.tone ?? "neutral";
  const percent = () => Math.max(0, Math.min(MAX, local.value));
  return (
    // biome-ignore lint/a11y/useAriaPropsSupportedByRole: the role is always progressbar or meter, which take a label
    <div
      role={local.meter ? "meter" : "progressbar"}
      aria-label={local.label}
      aria-valuenow={percent()}
      aria-valuemin={0}
      aria-valuemax={MAX}
      {...others}
      class={cx(
        "overflow-hidden rounded-full",
        local.size === SMALL ? "h-1" : "h-1.5",
        tone() === "ready" ? "bg-status-ready-subtle" : "bg-surface-sunken",
        local.meter && "border border-border",
        local.class,
      )}
    >
      <div
        class={cx("h-full transition-[width] duration-base ease-standard", FILLS[tone()])}
        style={{ width: `${percent()}%` }}
      />
    </div>
  );
}
