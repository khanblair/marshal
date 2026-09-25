import { type JSX, splitProps } from "solid-js";
import { cx } from "../base/cx";
import type { LooseString } from "../base/types";
import { type StatusKey, statusTone, toneSolidBg } from "./status";

const DEFAULT_DOT_SIZE_PX = 8;

export interface StatusDotProps extends Omit<JSX.HTMLAttributes<HTMLSpanElement>, "style"> {
  /** Card state, or `danger`. Unknown states use the neutral color. */
  state: StatusKey | "danger" | LooseString;
  /** Diameter in px. Default 8. */
  size?: number;
  style?: JSX.CSSProperties;
}

/**
 * Small round status marker in the state's solid color. It pulses only for
 * `working`. Port of the prototype's `m-dot`.
 */
export function StatusDot(props: StatusDotProps) {
  const [local, others] = splitProps(props, ["state", "size", "class", "style"]);
  const size = () => `${local.size ?? DEFAULT_DOT_SIZE_PX}px`;
  return (
    <span
      {...others}
      class={cx(
        "inline-block flex-none rounded-full",
        toneSolidBg[statusTone(local.state)],
        local.state === "working" && "animate-pulse-working",
        local.class,
      )}
      style={{ ...local.style, width: size(), height: size() }}
    />
  );
}
