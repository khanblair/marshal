import type { SelectOption } from "@marshal/ui";
import type { AgentOption } from "~/data/mappers/agents";

/**
 * The rows of an agent picker. An agent that is not installed stays in the list but cannot be
 * picked, and says so. `current` is the agent a card or role already has: when the catalog does
 * not know it (a mock card can name one), it is kept as the first row so the picker shows what
 * is really set instead of another agent.
 */
export function agentSelectOptions(
  agents: readonly AgentOption[],
  current?: string,
): SelectOption[] {
  const rows = agents.map((agent) => ({
    value: agent.name,
    label: agent.missing ? `${agent.name} (not installed)` : agent.name,
    disabled: agent.missing,
  }));
  const known = !current || agents.some((agent) => agent.name === current);
  return known ? rows : [{ value: current }, ...rows];
}

/**
 * The plain sentences to show near an agent picker, from the catalog: the warning of the chosen
 * agent when its version is untested, then how to install each agent that is missing.
 */
export function agentNotes(agents: readonly AgentOption[], selected?: string): string[] {
  const warning = agents.find((agent) => agent.name === selected)?.warning ?? "";
  const hints = agents.filter((agent) => agent.missing).map((agent) => agent.installHint);
  return [...new Set([warning, ...hints].filter((note) => note !== ""))];
}

/** The model rows for an agent: its models from the catalog, with the current one kept when the catalog does not list it. */
export function modelOptions(models: readonly string[], current: string): readonly string[] {
  return current === "" || models.includes(current) ? models : [current, ...models];
}
