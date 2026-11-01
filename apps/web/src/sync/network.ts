import type { NetworkStatus, Event as WireEvent } from "@marshal/protocol";
import { EventTypeNetworkChanged } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import type { Ctx } from "~/mock/context";
import type { Syncer } from "./syncer";

/**
 * Makes the store's network state the daemon's. The daemon checks the internet only after a call failed,
 * so the state is online until it says otherwise, and it says so on the home topic when it changes. A
 * snapshot and an event can describe the same change, so applying one twice redraws nothing.
 */
export function applyNetwork(ctx: Ctx, status: NetworkStatus): void {
  if (ctx.S.network.online !== status.online) ctx.S.network.online = status.online;
  if (ctx.S.network.since !== status.since) ctx.S.network.since = status.since;
}

/** Section S33: whether the internet is reachable. It follows the home topic for changes. */
export const networkSyncer: Syncer<NetworkStatus> = {
  section: "S33",
  topics: ["home"],
  load: (api: ApiClient) => api.network(),
  apply: applyNetwork,
  onEvent(ctx: Ctx, event: WireEvent): void {
    if (event.type !== EventTypeNetworkChanged) return;
    applyNetwork(ctx, event.data as NetworkStatus);
  },
};
