import { Button, Icon, ItemText } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M, type Profile } from "~/mock";

type Device = Profile["devices"][number];

/**
 * Paired devices, each with Remove device (B9.1, B9.2, build-plan 9.9).
 *
 * Everything about a device comes from the daemon: the list is its rows (section S2b), and removing
 * a device is the call that makes its token stop signing in. A revoked device stays in the list,
 * saying it was removed, rather than vanishing: a row that disappears leaves the person wondering
 * whether it is still signed in. Pairing a new one is `PairDevice`.
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

export function DevicesList() {
  return (
    <div class="flex flex-col border-t border-border">
      <Show when={M.S.profile.devices.filter((device) => !device.revoked).length === 0}>
        <p class="my-2.5 text-secondary">No paired devices.</p>
      </Show>
      <For each={M.S.profile.devices}>{(device) => <DeviceRow device={device} />}</For>
    </div>
  );
}
