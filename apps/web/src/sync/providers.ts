import type { ProviderList } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { type ProviderRows, toProviderRows } from "~/data/mappers/providers";
import type { Ctx } from "~/mock/context";
import type { Syncer } from "./syncer";

/**
 * Section S28: the provider keys. The daemon holds them in the OS keychain and answers with a
 * masked value only, so the store mirrors what the daemon says and never a key.
 *
 * There are no provider events: the routes answer the whole list, so a save, a remove, and a test's
 * re-read all arrive through `applyProviders`, and the snapshot this syncer loads is the same shape.
 * It publishes no topic, so it is loaded once when the app comes online and after every re-sync.
 */
export const providersSyncer: Syncer<ProviderRows> = {
  section: "S28",
  topics: [],
  async load(api: ApiClient) {
    return toProviderRows(await api.listProviders());
  },
  apply(ctx, rows) {
    applyProviders(ctx, rows);
  },
};

/** The daemon's list answer as the store holds it. Every provider route answers this shape. */
export function applyProviderList(ctx: Ctx, list: ProviderList): void {
  applyProviders(ctx, toProviderRows(list));
}

/** Replaces the store's rows with the daemon's. It is one write, so a screen redraws once. */
export function applyProviders(ctx: Ctx, rows: ProviderRows): void {
  ctx.S.providers = rows.providers;
}
