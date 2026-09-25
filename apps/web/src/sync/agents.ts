import {
  type Agent,
  type AgentCatalog,
  AgentKindBuiltin,
  AgentStatusSupported,
} from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import type { Ctx } from "~/mock/context";
import type { Syncer } from "./syncer";

/**
 * Marshal's own agent. The daemon's catalog does not list it until the built-in agent exists (build
 * plan Phase 4), but mock cards and roles still name it and the pickers show it in the design, so the
 * app adds this one entry after the daemon's agents. It is the only agent the app makes up: it is
 * removed here when the daemon lists it. The models are the prototype's; the first is the default.
 */
export const BUILT_IN_AGENT: Agent = {
  kind: AgentKindBuiltin,
  name: "Built-in agent",
  version: "Marshal 0.9",
  status: AgentStatusSupported,
  warning: "",
  installHint: "",
  models: [
    { id: "claude-sonnet-4-5", name: "Claude Sonnet 4.5", thinking: true },
    { id: "gpt-5-mini", name: "GPT-5 mini", thinking: true },
    { id: "deepseek-chat", name: "DeepSeek Chat", thinking: false },
    { id: "gemini-2.5-flash", name: "Gemini 2.5 Flash", thinking: true },
    { id: "qwen2.5-coder:32b", name: "Qwen2.5 Coder 32B", thinking: false },
  ],
  capabilities: {
    resume: true,
    structuredEvents: true,
    modelSwitching: true,
    thinking: true,
    mcp: true,
    approvals: true,
  },
};

/** The catalog the screens use: the daemon's agents in the daemon's order, then the built-in agent. */
export function withBuiltIn(agents: readonly Agent[]): Pick<AgentCatalog, "agents"> {
  const listed = agents.some((agent) => agent.kind === AgentKindBuiltin);
  return { agents: listed ? [...agents] : [...agents, BUILT_IN_AGENT] };
}

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
