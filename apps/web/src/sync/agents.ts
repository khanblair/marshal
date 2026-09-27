import type { Agent, AgentCatalog } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import type { Ctx } from "~/mock/context";
import type { Syncer } from "./syncer";

const sameAgents = (a: readonly Agent[], b: readonly Agent[]): boolean =>
  JSON.stringify(a) === JSON.stringify(b);

/**
 * Makes the store's agent catalog the daemon's. The list is replaced only when it differs, so
 * applying the same catalog twice (a snapshot and a `Resync` both do) redraws nothing.
 */
export function applyAgentCatalog(ctx: Ctx, catalog: AgentCatalog): void {
  if (sameAgents(ctx.S.agents, catalog.agents)) return;
  ctx.S.agents = structuredClone(catalog.agents);
}

/** Section S4: the agent catalog, loaded on connect and on every return, with no events of its own. */
export const agentsSyncer: Syncer<AgentCatalog> = {
  section: "S4",
  topics: [],
  load: (api: ApiClient) => api.agents(),
  apply: applyAgentCatalog,
};
