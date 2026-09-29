import { Button, Icon, ItemText } from "@marshal/ui";
import { createResource, For, Show } from "solid-js";
import { M, type Profile } from "~/mock";
import { pairingPayload, tailnetHost } from "~/platform/pairing";
import { PairingQr } from "./PairingQr";
import type { ProfileDraft } from "./use-profile-draft";

type Device = Profile["devices"][number];

/**
 * Paired devices, each with Remove device, and the Pair a device button with its own code
 * (B9.1, B9.2, build-plan 9.9).
 *
 * Everything about a device now comes from the daemon: the list is its rows (section S2b), the code
 * is one the daemon just made and that expires in five minutes, and removing a device is the call
 * that makes its token stop signing in. The prototype's hardcoded code is gone - the only thing
 * shown here is what the daemon actually answered.
 *
 * A revoked device stays in the list, saying it was removed, rather than vanishing: a row that
 * disappears leaves the person wondering whether it is still signed in.
 */

function confirmRemove(device: Device): void {
  M.confirm({
    title: "Remove device",
    message: `${device.name} will lose access to Marshal and must pair again to reconnect.`,
    action: "Remove device",
    destructive: true,
    run: () => {
      void M.removeDevice(device.id).then((removed) => {
        if (removed) M.toast("Device removed");
      });
    },
  });
}

/**
 * The icon a device kind is drawn with. The daemon's kinds are its own words (`mobile`, `web`,
 * `desktop`, `cli`, `dev`) and the design's icons are drawn by the names in ui, so the two are
 * matched here rather than the wire type being bent to fit an icon set. An unknown kind falls back
 * to a phone, because a paired device is most often one.
 */
function deviceIcon(kind: string): string {
  switch (kind) {
    case "web":
      return "globe";
    case "desktop":
      return "monitor";
    case "cli":
    case "dev":
      return "terminal";
    default:
      return "smartphone";
  }
}

function DeviceRow(props: { device: Device }) {
  const revoked = () => props.device.revoked === true;
  return (
    <div class="flex flex-wrap items-center gap-x-3 gap-y-2 py-2.5 border-b border-border">
      <Icon name={deviceIcon(props.device.kind)} size={18} />
      <ItemText
        basis={160}
        title={props.device.name}
        description={
          revoked()
            ? "Removed - pair again to reconnect"
            : `Last seen ${M.rel(props.device.last).toLowerCase()}`
        }
      />
      <Show when={!revoked()}>
        <Button tone="danger" onClick={() => confirmRemove(props.device)}>
          Remove device
        </Button>
      </Show>
    </div>
  );
}

export function DevicesList(props: { profile: ProfileDraft }) {
  // Asking for a code replaces the one that was live: only one code is ever on the screen, and the
  // old one stops working the moment a new one is made (B9.1).
  const pair = (): void => {
    props.profile.showPairingCode();
    void M.createPairingCode();
  };
  const code = () => M.pairingCode();
  // The QR carries the address a phone reaches this daemon at, which only a node that is online
  // has. Without one the text code still works for a device that already knows the address.
  const [node] = createResource(async () => await M.tailnetStatus());
  const qr = () => {
    const status = node();
    const live = code();
    if (!live || status?.state !== "online" || !status.dnsName) return null;
    return pairingPayload(tailnetHost(status.dnsName, window.location.port), live.code);
  };
  return (
    <div class="flex flex-col border-t border-border">
      <Show when={M.S.profile.devices.filter((device) => !device.revoked).length === 0}>
        <p class="my-2.5 text-secondary">No paired devices.</p>
      </Show>
      <For each={M.S.profile.devices}>{(device) => <DeviceRow device={device} />}</For>
      <div class="flex flex-wrap items-center gap-3 py-3">
        <Button onClick={pair}>Pair a device</Button>
        <Show when={props.profile.pairing() && code()}>
          <span class="flex items-center gap-2">
            <span class="text-small text-secondary">Enter this code on the device</span>
            <code class="font-mono text-title font-semibold py-0.5 px-2 rounded-sm bg-surface-sunken">
              {code()?.code ?? ""}
            </code>
          </span>
        </Show>
      </div>
      <Show when={props.profile.pairing() && qr()}>
        {(payload) => (
          <div class="flex flex-wrap items-center gap-3 pb-3">
            <PairingQr text={payload()} label="Pairing QR code" />
            <span class="text-small text-secondary max-w-60">
              In the Marshal phone app, choose Scan the QR code.
            </span>
          </div>
        )}
      </Show>
    </div>
  );
}
