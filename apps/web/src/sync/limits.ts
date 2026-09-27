import type { LimitList } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { type LimitsByScope, toLimitsByScope } from "~/data/mappers/limits";
import type { Ctx } from "~/mock/context";
import type { Syncer } from "./syncer";

/**
 * Section S26b: the cost and awake limits. The daemon holds them, so the store mirrors what it says.
 *
 * There are no limit events: the routes answer the whole list, so a load, a save, and a remove all
 * arrive through `applyLimitList`, and the snapshot this syncer loads is the same shape. It publishes
 * no topic, so it is loaded once when the app comes online and after every re-sync.
 */
export const limitsSyncer: Syncer<LimitsByScope> = {
  section: "S26b",
  topics: [],
  async load(api: ApiClient) {
    return toLimitsByScope(await api.listLimits());
  },
  apply(ctx, limits) {
    applyLimitsByScope(ctx, limits);
  },
};

/** The daemon's list answer as the store holds it. Every limits route answers this shape. */
export function applyLimitList(ctx: Ctx, list: LimitList): void {
  applyLimitsByScope(ctx, toLimitsByScope(list));
}

/** Replaces the store's ceilings with the daemon's. It is one write, so a screen redraws once. */
export function applyLimitsByScope(ctx: Ctx, limits: LimitsByScope): void {
  ctx.S.limits = limits;
}
