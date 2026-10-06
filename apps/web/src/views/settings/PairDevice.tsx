import type { TailnetStatus } from "@marshal/protocol";
import { Button } from "@marshal/ui";
import { Show } from "solid-js";
import { M } from "~/mock";
import { pairingQrText } from "~/platform/pairing";
import { phoneAccess } from "~/platform/phone-access";
import { PairingQr } from "./PairingQr";
import type { RemoteDraft } from "./use-remote-draft";

const SUBHEADING = "mt-2 mb-0 text-subtitle leading-5.5 font-semibold";

/**
 * Pair a device: a code the daemon just made, which stops working after five minutes, and a QR code
 * that carries the address and the code once a phone has an address that works. Asking for a code
 * replaces the one that was live, so only one is ever on the screen.
 */
export function PairDevice(props: {
  draft: RemoteDraft;
  status: TailnetStatus | null | undefined;
}) {
  const code = () => M.pairingCode();
  const qr = () => pairingQrText(props.status, code()?.code);
  // Only a daemon that answered can say why there is no QR code. A prototype has nothing to say.
  const noQr = () => Boolean(phoneAccess(props.status)) && !qr();
  const pair = (): void => {
    props.draft.showPairingCode();
    void M.createPairingCode();
  };
  return (
    <>
      <h3 class={SUBHEADING}>Pair a device</h3>
      <div class="flex flex-wrap items-center gap-3">
        <Button onClick={pair}>Pair a device</Button>
        <Show when={props.draft.pairing() && code()}>
          <span class="flex items-center gap-2">
            <span class="text-small text-secondary">Enter this code on the device</span>
            <code class="font-mono text-title font-semibold py-0.5 px-2 rounded-sm bg-surface-sunken">
              {code()?.code ?? ""}
            </code>
          </span>
        </Show>
      </div>
      <Show when={props.draft.pairing() && code() && noQr()}>
        <p class="m-0 text-small leading-4.5 text-secondary max-w-[65ch]">
          A QR code appears once the phone address above works. Until then, type the code on a
          device that already knows this computer's address.
        </p>
      </Show>
      <Show when={props.draft.pairing() && qr()}>
        {(payload) => (
          <div class="flex flex-wrap items-center gap-3">
            <PairingQr text={payload()} label="Pairing QR code" />
            <span class="text-small text-secondary max-w-60">
              In the Marshal phone app, choose Scan the QR code.
            </span>
          </div>
        )}
      </Show>
    </>
  );
}
