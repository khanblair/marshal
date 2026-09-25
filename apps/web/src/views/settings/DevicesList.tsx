import { Button, Icon, ItemText } from "@marshal/ui";
import { batch, For, Show } from "solid-js";
import { M, type Profile } from "~/mock";
import type { ProfileDraft } from "./use-profile-draft";

type Device = Profile["devices"][number];

/** The code the design shows when you pair a device. */
const PAIRING_CODE = "7QX-2LD";

function confirmRemove(device: Device): void {
  M.confirm({
    title: "Remove device",
    message: `${device.name} will lose access to Marshal and must pair again to reconnect.`,
    action: "Remove device",
    destructive: true,
    run: () =>
      batch(() => {
        M.S.profile.devices = M.S.profile.devices.filter((other) => other.id !== device.id);
        M.toast("Device removed");
      }),
  });
}

function DeviceRow(props: { device: Device }) {
  return (
    <div class="flex flex-wrap items-center gap-x-3 gap-y-2 py-2.5 border-b border-border">
      <Icon name={props.device.kind} size={18} />
      <ItemText
        basis={160}
        title={props.device.name}
        description={`Last seen ${M.rel(props.device.last).toLowerCase()}`}
      />
      <Button tone="danger" onClick={() => confirmRemove(props.device)}>
        Remove device
      </Button>
    </div>
  );
}

/** Paired devices, each with Remove device, and the Pair a device button with its code. */
export function DevicesList(props: { profile: ProfileDraft }) {
  return (
    <div class="flex flex-col border-t border-border">
      <Show when={M.S.profile.devices.length === 0}>
        <p class="my-2.5 text-secondary">No paired devices.</p>
      </Show>
      <For each={M.S.profile.devices}>{(device) => <DeviceRow device={device} />}</For>
      <div class="flex flex-wrap items-center gap-3 py-3">
        <Button onClick={props.profile.showPairingCode}>Pair a device</Button>
        <Show when={props.profile.pairing()}>
          <span class="flex items-center gap-2">
            <span class="text-small text-secondary">Enter this code on the device</span>
            <code class="font-mono text-title font-semibold py-0.5 px-2 rounded-sm bg-surface-sunken">
              {PAIRING_CODE}
            </code>
          </span>
        </Show>
      </div>
    </div>
  );
}
