import { type Card, type Column, M, type SwimKey } from "~/mock";
import { ALL_LANE, NO_PACKAGE } from "./board-model";

/** What a swimlane adds to a card created inside it. */
export type LaneExtra = Partial<Pick<Card, "role" | "agent" | "pkg" | "model">>;

export function laneExtraOf(swim: SwimKey | undefined, laneKey: string): LaneExtra {
  if (swim === "role" && laneKey !== ALL_LANE) return { role: laneKey };
  if (swim === "agent") return { agent: laneKey };
  if (swim === "package" && laneKey !== NO_PACKAGE) return { pkg: laneKey };
  return {};
}

/** A lane of an agent that is not installed cannot give a card that agent, so the card keeps the default one. */
function withoutMissingAgent(extra: LaneExtra): LaneExtra {
  const lane = M.agentOptions().find((agent) => agent.name === extra.agent);
  if (!lane?.missing) return extra;
  const { agent: _missing, ...rest } = extra;
  return rest;
}

/**
 * Adds a card from the inline form, with the lane's role, agent, package, and model. An empty title
 * does nothing. The prototype throws for an agent lane it has no model list for; the port keeps the
 * card's model instead.
 *
 * The lane's fields go with the create, because a card is one request and the cards now come from
 * the daemon: a create that has not answered yet cannot be found by looking at the last card in the
 * store, which is what this used to do.
 */
export function submitQuickAdd(pid: string, col: Column, title: string, lane: LaneExtra): void {
  if (!title.trim()) return;
  const extra = withoutMissingAgent(lane);
  // The catalog says which models the lane's agent has. The first is the agent's default.
  const model = extra.agent ? M.AGENTS[extra.agent]?.models[0] : undefined;
  M.quickAdd(pid, col, title, model ? { ...extra, model } : extra);
}

/** Opens the New card dialog on the Bug fix template, starting the card unless it is Backlog. */
export function openTemplateCard(col: Column, lane: LaneExtra): void {
  const extra = withoutMissingAgent(lane);
  M.newCard({
    template: "Bug fix",
    start: col !== "backlog",
    ...(extra.role ? { role: extra.role } : {}),
    ...(extra.agent ? { agent: extra.agent } : {}),
  });
}
