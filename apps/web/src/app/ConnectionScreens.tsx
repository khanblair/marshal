import { ConnectionLost, ErrorState, OfflineBanner, SignIn } from "@marshal/ui";
import { createSignal, onCleanup, onMount } from "solid-js";
import { M } from "~/mock";
import { platform } from "~/platform";
import { parsePairingScan } from "~/platform/pairing";
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
  const [pairing, setPairing] = createSignal(false);
  const [pairError, setPairError] = createSignal("");
  const device = platform();
  const pair = (code: string, name: string) => {
    setPairing(true);
    setPairError("");
    void M.pairWithCode(code, name)
      .then(setPairError)
      .finally(() => setPairing(false));
  };
  const deviceName = () => {
    if (device.kind === "mobile") return "Phone app";
    return isPhone() ? "Phone browser" : "Web browser";
  };
  const scan = () => {
    void device.scanCode().then((text) => {
      const code = parsePairingScan(text);
      if (code) pair(code, deviceName());
      else if (text)
        setPairError("That is not a Marshal code. Scan the one shown in Pair a device.");
      else
        setPairError(
          "The camera could not read a code. Allow the camera for Marshal, or type the code instead.",
        );
    });
  };
  // A phone that opened the address from the QR code arrives with the code in it. It is taken out
  // of the address at once, so it is not left in the history or a bookmark.
  onMount(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("pair");
    if (!code) return;
    params.delete("pair");
    const rest = params.toString();
    window.history.replaceState(null, "", window.location.pathname + (rest ? `?${rest}` : ""));
    pair(code, deviceName());
  });
  return (
    <SignIn
      phone={isPhone()}
      busy={M.S.connection?.busy || pairing()}
      error={M.S.connection?.rejection}
      onSubmit={(token) => M.signIn(token)}
      onPair={pair}
      onScan={device.canScanCode ? scan : undefined}
      pairError={pairError()}
      deviceName={deviceName()}
    />
  );
}

/** The bar for a connection that dropped while the app has data. */
export function OfflineNotice() {
  const seconds = createCountdown(() => M.S.connection?.retryAt);
  const updatedAt = () => M.S.connection?.updatedAt;
  return (
    <OfflineBanner
      retryInSeconds={seconds()}
      machine={M.S.profile.node || "your computer"}
      updatedAgo={updatedAt() ? M.rel(updatedAt() as number).toLowerCase() : undefined}
    />
  );
}

/** The first snapshots could not be loaded, though the daemon answers. */
export function LoadFailedScreen(props: { message: string }) {
  return (
    <div class="flex-1 min-w-0 flex items-center justify-center p-6">
      <ErrorState message={props.message} onRetry={() => M.reconnect()} />
    </div>
  );
}
