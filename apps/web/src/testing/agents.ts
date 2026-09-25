/**
 * Typed builders for the agent catalog as the daemon sends it. They start from the golden
 * `agents.json` that the Go tests write, so a fixture cannot drift from the wire shape, and change
 * only the fields a test cares about.
 */
import type { Agent, AgentCatalog } from "@marshal/protocol";
import { golden } from "~/data/testing/golden";

/** The catalog the Go tests write: Claude Code supported, Gemini CLI untested, and Codex missing. */
export const GOLDEN_CATALOG: AgentCatalog = golden<AgentCatalog>("agents");

/** One agent on the wire: the golden Claude Code, with the given fields changed. */
export function wireAgent(fields: Partial<Agent> & { kind: Agent["kind"] }): Agent {
  const [base] = GOLDEN_CATALOG.agents;
  if (!base) throw new Error("the golden catalog has no agents");
  return { ...base, ...fields };
}

/** A catalog answer with these agents, in this order. */
export const wireCatalog = (agents: readonly Agent[]): AgentCatalog => ({
  agents: [...agents],
  serverTime: GOLDEN_CATALOG.serverTime,
});

const ALL_CAPABILITIES = {
  resume: true,
  structuredEvents: true,
  modelSwitching: true,
  thinking: true,
  mcp: true,
  approvals: true,
};

const thinking = (ids: readonly string[]) => ids.map((id) => ({ id, name: id, thinking: true }));

/**
 * The agents of the prototype's fixed table, as a catalog that reports them all as installed and
 * supported, with every capability on: same names, order, versions, and models. The unit tests
 * use it so their screens read what the prototype's did, and the tests of the missing and untested
 * states use the golden catalog instead.
 */
export const PROTOTYPE_CATALOG: AgentCatalog = wireCatalog([
  wireAgent({
    kind: "claude",
    name: "Claude Code",
    version: "2.0.14",
    status: "supported",
    warning: "",
    installHint: "",
    models: thinking(["claude-sonnet-4-5", "claude-opus-4-1", "claude-haiku-4-5"]),
    capabilities: ALL_CAPABILITIES,
  }),
  wireAgent({
    kind: "codex",
    name: "Codex",
    version: "0.42.0",
    status: "supported",
    warning: "",
    installHint: "",
    models: thinking(["gpt-5-codex", "gpt-5", "gpt-5-mini"]),
    capabilities: ALL_CAPABILITIES,
  }),
  wireAgent({
    kind: "gemini",
    name: "Gemini CLI",
    version: "0.8.1",
    status: "supported",
    warning: "",
    installHint: "",
    models: thinking(["gemini-2.5-pro", "gemini-2.5-flash"]),
    capabilities: ALL_CAPABILITIES,
  }),
]);
