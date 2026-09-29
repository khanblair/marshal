import {
  type Device as WireDevice,
  type DeviceList,
  type PairingCode,
  type TailnetPeerList,
  type TailnetStatus,
} from "@marshal/protocol";
import { createSignal } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import type { Ctx } from "~/mock/context";
import type { Syncer } from "./syncer";

/*
 * Section S2b: the devices signed in with this person, and the code a new one is paired with
 * (B9.1, B9.2, build-plan 9.9). The daemon owns the rows: it holds the token hash, and revoking one
 * is what makes it stop working, so the store mirrors the list rather than the other way round.
 *
 * The list has no events of its own - the route answers everything, and a load, a pair, and a
 * revoke all read the same call - so it publishes no topic and is read once when the app comes
 * online, the way the limits and the sleep settings are.
 *
 * The pairing code is not part of the store: it is shown for one press, expires in five minutes, and
 * is never a state a screen redraws from, so it lives beside the action that makes it.
 */

/** The device rows the store holds, as the paired-devices list draws them. */
function toDeviceRow(device: WireDevice): {
  id: string;
  name: string;
  kind: string;
  last: number;
  revoked: boolean;
} {
  // A device that has not called yet has no last-seen time, so the pairing time is what the row
  // shows: it is the moment this device became known, which is a true answer rather than nothing.
  const seen = device.lastSeenAt ?? device.pairedAt;
  return {
    id: device.id,
    name: device.name,
    kind: device.kind,
    last: Date.parse(seen),
    revoked: device.revoked,
  };
}

/** Writes the daemon's list into the store, in its own order, revoked devices included. */
export function applyDevices(ctx: Ctx, list: DeviceList): void {
  ctx.S.profile.devices = list.devices.map(toDeviceRow);
}

export const devicesSyncer: Syncer<DeviceList> = {
  section: "S2b",
  topics: [],
  async load(api: ApiClient) {
    return api.listDevices();
  },
  apply(ctx, list) {
    applyDevices(ctx, list);
  },
};

/** The code the last press of "Pair a device" made, and when it stops working. */
const [pairingCode, setPairingCode] = createSignal<PairingCode | null>(null);

/**
 * The live pairing code, or null before one is asked for or after it expired. It takes the store
 * context the way every other member of the app's `M` surface does, and ignores it: the code is
 * about the daemon, not about this store.
 */
export function currentPairingCode(_ctx: Ctx): PairingCode | null {
  return pairingCode();
}

/**
 * Asks the daemon for a fresh pairing code, replacing any code that was live. The code is short by
 * design (five minutes, single use), so a second press makes a second one and the first stops
 * working - which is the point: only one code is on the screen at a time.
 */
export async function requestPairingCode(c: Ctx): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  try {
    setPairingCode(await api.createPairingCode());
    return true;
  } catch {
    return false;
  }
}

/** Forgets the live pairing code, so nothing is left on the screen after the section is left. */
export function clearPairingCode(): void {
  setPairingCode(null);
}

/**
 * What the daemon's own node on the tailnet is doing (B9.1, build-plan 9.9). It answers "off" at
 * once on a daemon that was never started with `--tailnet`, so the status never waits, and a store
 * with no daemon answers nothing at all - which is a mock section showing its own words instead.
 */
export async function readTailnetStatus(c: Ctx): Promise<TailnetStatus | null> {
  const api = c.env.data?.api;
  if (!api) return null;
  try {
    return await api.tailnetStatus();
  } catch {
    return null;
  }
}

/**
 * The other machines on this tailnet and which of them are Marshal daemons (B9.5). It reads the
 * network every time rather than keeping an answer, because a machine joining or leaving is the
 * thing a person opened the screen to see, and a store with no daemon answers nothing at all.
 */
export async function readTailnetPeers(c: Ctx): Promise<TailnetPeerList | null> {
  const api = c.env.data?.api;
  if (!api) return null;
  try {
    return await api.tailnetPeers();
  } catch {
    return null;
  }
}

/**
 * Revokes one device, and reads the list again so the row is marked revoked rather than vanishing -
 * the screen says a device was removed rather than watching it disappear (B9.1).
 */
export async function removeDevice(c: Ctx, id: string): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  try {
    await api.removeDevice(id);
    try {
      applyDevices(c, await api.listDevices());
    } catch {
      // The revoke already went through; only the re-read failed, so the list keeps what it had.
    }
    return true;
  } catch {
    return false;
  }
}
