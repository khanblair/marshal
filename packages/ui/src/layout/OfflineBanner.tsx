import { type JSX, Show, splitProps } from "solid-js";
import { RetryCountdown } from "../base/_RetryCountdown";
import { Callout } from "../base/Callout";
import { cx } from "../base/cx";
import { Icon } from "../icons/Icon";

export interface OfflineBannerProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /** Seconds until the next automatic try. Shows "Trying again in 4 s"; nothing when null. */
  retryInSeconds?: number | null;
  /** The machine that cannot be reached, such as "Office PC". Without it the bar just says offline. */
  machine?: string;
  /** How long ago the data on screen was fresh, such as "5 min ago". Only shown with a machine. */
  updatedAgo?: string;
}

const ICON_PX = 14;

/**
 * A thin bar at the top of the app for when it already has data and the connection dropped.
 * It does not promise that changes are kept: there are no queued offline actions, so it says
 * changes cannot be made until Marshal reconnects. One line on desktop; on a phone the text
 * wraps inside the bar and never widens the page. It announces politely; the countdown is
 * marked `aria-live="off"` so its ticks are not read out.
 */
export function OfflineBanner(props: OfflineBannerProps) {
  const [local, others] = splitProps(props, ["retryInSeconds", "machine", "updatedAgo", "class"]);
  return (
    <Callout
      aria-live="polite"
      {...others}
      class={cx("flex-none rounded-none! py-1.5! px-4!", local.class)}
    >
      <Icon name="triangle-alert" size={ICON_PX} class="mt-0.5" />
      <span class="min-w-0">
        <Show when={local.machine} fallback="You're offline. ">
          Can't reach {local.machine}.{" "}
          <Show when={local.updatedAgo}>Last updated {local.updatedAgo}. </Show>
        </Show>
        Marshal reconnects on its own. Changes can't be made until then.{" "}
        <RetryCountdown seconds={local.retryInSeconds} aria-live="off" />
      </span>
    </Callout>
  );
}
