import { ConnectionLost, ErrorState, OfflineBanner, SignIn } from "@marshal/ui";
import { createSignal, onCleanup } from "solid-js";
import { M } from "~/mock";
import { isPhone } from "./shell-layout";

const TICK_MS = 1000;
const MS_PER_SECOND = 1000;

/**
 * Whole seconds until `retryAt` (a time on this device's clock), counted down once a second, or null
 * when no try is planned. It is the local clock on purpose: the daemon may be gone, and its time
 * says nothing about when this device will ask again.
 */
function createCountdown(retryAt: () => number | null | undefined): () => number | null {
  const [now, setNow] = createSignal(Date.now());
  const timer = setInterval(() => setNow(Date.now()), TICK_MS);
  onCleanup(() => clearInterval(timer));
  return () => {
    const at = retryAt();
    return at == null ? null : Math.max(0, Math.ceil((at - now()) / MS_PER_SECOND));
  };
}

/** The whole app has no data and the daemon does not answer. It closes by itself when it does. */
export function ConnectionLostScreen() {
  const seconds = createCountdown(() => M.S.connection?.retryAt);
  return (
    <ConnectionLost
      phone={isPhone()}
      retryInSeconds={seconds()}
      details={M.S.connection?.detail}
      onRetry={() => M.reconnect()}
    />
  );
}

/** The daemon does not know this device. The token is kept only by the token store, never here. */
export function SignInScreen() {
  return (
    <SignIn
      phone={isPhone()}
      busy={M.S.connection?.busy}
      error={M.S.connection?.rejection}
      onSubmit={(token) => M.signIn(token)}
    />
  );
}

/** The bar for a connection that dropped while the app has data. */
export function OfflineNotice() {
  const seconds = createCountdown(() => M.S.connection?.retryAt);
  return <OfflineBanner retryInSeconds={seconds()} />;
}

/** The first snapshots could not be loaded, though the daemon answers. */
export function LoadFailedScreen(props: { message: string }) {
  return (
    <div class="flex-1 min-w-0 flex items-center justify-center p-6">
      <ErrorState message={props.message} onRetry={() => M.reconnect()} />
    </div>
  );
}
