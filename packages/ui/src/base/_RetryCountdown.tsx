import { type JSX, Show, splitProps } from "solid-js";
import { cx } from "./cx";

/* Private: the "Trying again in 4 s" line of ConnectionLost and OfflineBanner. */

interface RetryCountdownProps extends JSX.HTMLAttributes<HTMLSpanElement> {
  /** Seconds until the next try. Nothing shows while it is null or missing; 0 still shows. */
  seconds?: number | null;
}

/** Whole seconds, never negative, so a late timer tick cannot print "-1 s". */
function wholeSeconds(seconds: number | null | undefined): number | null {
  return seconds == null ? null : Math.max(0, Math.ceil(seconds));
}

/**
 * The time until the next try, kept on one line so the number never parts from its unit.
 * Keep it out of a live region, or a screen reader would read it out every second.
 */
export function RetryCountdown(props: RetryCountdownProps) {
  const [local, others] = splitProps(props, ["seconds", "class"]);
  const seconds = () => wholeSeconds(local.seconds);
  return (
    <Show when={seconds() !== null}>
      <span {...others} class={cx("whitespace-nowrap", local.class)}>
        Trying again in {seconds()} s
      </span>
    </Show>
  );
}
