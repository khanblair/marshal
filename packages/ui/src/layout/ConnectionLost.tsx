import { type JSX, Show, splitProps } from "solid-js";
import { Details } from "../base/_Details";
import { RetryCountdown } from "../base/_RetryCountdown";
import { Button } from "../base/Button";
import { Icon } from "../icons/Icon";
import { ScreenFrame } from "./_ScreenFrame";

export interface ConnectionLostProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /** Called when the person presses Try again. */
  onRetry: () => void;
  /** Seconds until the next automatic try. Shows "Trying again in 4 s"; nothing when null. */
  retryInSeconds?: number | null;
  /** Technical text (an error code and a short reason), shown only inside "Details". */
  details?: string;
  /** Phone layout. */
  phone?: boolean;
}

const ICON_PX = 20;

/**
 * The full screen for a daemon that cannot be reached: the app has no data yet, or it has some and
 * the daemon has stayed silent. It says what to check, offers Try again, and closes when the parent
 * stops showing it. The message is announced when it appears (`role="alert"`); the countdown sits
 * outside it so it is not read out again every second. While the app is only reconnecting, use
 * `OfflineBanner` instead (`docs/ui-rules.md` 5.6).
 */
export function ConnectionLost(props: ConnectionLostProps) {
  const [local, others] = splitProps(props, ["onRetry", "retryInSeconds", "details", "phone"]);
  return (
    <ScreenFrame phone={local.phone} {...others}>
      <div role="alert" class="flex flex-col items-center gap-3">
        <Icon name="server" size={ICON_PX} />
        <h1 class="m-0 text-title leading-6 font-semibold">Can't reach the daemon</h1>
        <p class="m-0 text-secondary">
          Marshal runs as a background service on this computer. Check that it is running. This
          screen closes by itself when it answers.
        </p>
      </div>
      <Button variant="primary" size={36} onClick={() => local.onRetry()}>
        Try again
      </Button>
      <RetryCountdown seconds={local.retryInSeconds} class="text-small text-muted" />
      <Show when={local.details}>{(text) => <Details text={text()} />}</Show>
    </ScreenFrame>
  );
}
