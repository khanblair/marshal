import type {
  Agent,
  AgentCapabilities,
  AgentCatalog,
  AgentKind,
  AgentModel,
  AgentStatus,
} from "@marshal/protocol";

/** One entry of the agent picker. An agent that is not installed is here too, marked as missing. */
export interface AgentOption {
  kind: AgentKind;
  /** The words shown to people, such as "Claude Code". */
  name: string;
  version: string;
  status: AgentStatus;
  /** True when the agent is not installed. The row is shown but cannot be picked. */
  missing: boolean;
  /** One plain sentence when there is something to know, such as an untested version. Else empty. */
  warning: string;
  /** One plain sentence with the install command, for a missing agent. Else empty. */
  installHint: string;
  models: AgentModel[];
  capabilities: AgentCapabilities;
}

/**
 * One agent in the shape of the prototype's `AGENTS` table, which the pickers and the card
 * settings read. It lives in the data layer, not in `mock/`, so the mapper never depends on the
 * mock; `mock/types.ts` re-exports it for the store's own use.
 */
export interface AgentInfo {
  models: string[];
  icon: string;
  version: string;
}

/** What the mappers read of the catalog: its agents. The daemon's answer has the server time too, and the store keeps only the agents. */
export type CatalogAgents = Pick<AgentCatalog, "agents"> &
  Partial<Pick<AgentCatalog, "serverTime">>;

/* The icons the mock's `AGENTS` table uses, so the pickers look the same on either side. */
const AGENT_ICONS: Record<AgentKind, string> = {
  claude: "terminal-square",
  gemini: "terminal-square",
  codex: "terminal-square",
  builtin: "cpu",
};

function toOption(agent: Agent): AgentOption {
  return {
    kind: agent.kind,
    name: agent.name,
    version: agent.version,
    status: agent.status,
    missing: agent.status === "missing",
    warning: agent.warning,
    installHint: agent.installHint,
    models: agent.models.map((model) => ({ ...model })),
    capabilities: { ...agent.capabilities },
  };
}

/** The catalog as a list for the pickers, in the order the daemon gives, with no agent dropped. */
export function toAgentOptions(catalog: CatalogAgents): AgentOption[] {
  return catalog.agents.map(toOption);
}

/**
 * The catalog in the shape of the mock's `AGENTS`: keyed by the name shown to people, with the
 * model ids (what a card sends back to the daemon), an icon, and the version. The built-in agent
 * is not in the daemon's catalog, so it has no key here. A missing agent has one, with an empty version.
 */
export function toLegacyAgents(catalog: CatalogAgents): Record<string, AgentInfo> {
  return Object.fromEntries(
    catalog.agents.map((agent) => [
      agent.name,
      {
        models: agent.models.map((model) => model.id),
        icon: AGENT_ICONS[agent.kind],
        version: agent.version,
      },
    ]),
  );
}

/**
 * A function like the mock's `thinkSupported`: does the thinking picker apply to this model? The
 * model must have a thinking setting, and the agent must let Marshal set it, because an agent may
 * have models that think and still give Marshal no way to say how hard (`AgentModel.Thinking` in
 * the Go types). A model the catalog does not list has none.
 */
export function thinkSupportedIn(catalog: CatalogAgents): (model: string) => boolean {
  const supported = new Set(
    catalog.agents
      .filter((agent) => agent.capabilities.thinking)
      .flatMap((agent) => agent.models.filter((model) => model.thinking).map((model) => model.id)),
  );
  return (model) => supported.has(model);
}
