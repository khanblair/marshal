import {
  type AgentOption,
  thinkSupportedIn,
  toAgentOptions,
  toLegacyAgents,
} from "~/data/mappers/agents";
import { withBuiltIn } from "~/sync/agents";
import type { Ctx } from "./context";
import type { AgentInfo } from "./types";

/* Queries on the agent catalog. It is mirrored from the daemon into `S.agents` (`sync/agents.ts`);
   these read it, with the built-in agent added after the daemon's own, so the pickers, the card
   settings, and the mock's own actions all see the same list. A name or model the catalog does not
   know is never an error: it reads as "no models" and "no thinking". */

const PREFERRED_AGENT = "Claude Code";

const catalogOf = (ctx: Ctx) => withBuiltIn(ctx.S.agents);

/** Every agent a card can use, in the order to show them: the daemon's, then the built-in agent. A missing agent is in it, marked. */
export const agentOptions = (ctx: Ctx): AgentOption[] => toAgentOptions(catalogOf(ctx));

/** The catalog in the shape of the prototype's `AGENTS` table: by name, with model ids, an icon, and a version. */
export const agentTable = (ctx: Ctx): Record<string, AgentInfo> => toLegacyAgents(catalogOf(ctx));

/** First model of an agent, which is what a card or role gets when it changes to that agent. */
export const defaultModel = (ctx: Ctx, agent: string): string | undefined =>
  agentTable(ctx)[agent]?.models[0];

/**
 * Whether the thinking picker applies to a model: the model has a thinking setting and its agent
 * lets Marshal set it. A model that no agent lists has none.
 */
export const thinkSupported = (ctx: Ctx, model: string): boolean =>
  thinkSupportedIn(catalogOf(ctx))(model);

/** The models that the catalog lists and that have no thinking setting, which the prototype kept as a fixed list. */
export function noThink(ctx: Ctx): string[] {
  const supported = thinkSupportedIn(catalogOf(ctx));
  const known = agentOptions(ctx).flatMap((agent) => agent.models.map((model) => model.id));
  return [...new Set(known)].filter((id) => !supported(id));
}

/** The agent a new card starts with: Claude Code while it can be used, else the first agent that can. */
export function defaultAgentOf(agents: readonly AgentOption[]): string {
  const usable = agents.filter((agent) => !agent.missing);
  return (
    usable.find((agent) => agent.name === PREFERRED_AGENT)?.name ??
    usable[0]?.name ??
    PREFERRED_AGENT
  );
}

export const defaultAgent = (ctx: Ctx): string => defaultAgentOf(agentOptions(ctx));
