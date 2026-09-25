import { batch } from "solid-js";
import { type Card, type Column, M, type SwimKey } from "~/mock";
import { ALL_LANE, NO_PACKAGE } from "./board-model";

/** What a swimlane adds to a card created inside it. */
export type LaneExtra = Partial<Pick<Card, "role" | "agent" | "pkg">>;

export function laneExtraOf(swim: SwimKey | undefined, laneKey: string): LaneExtra {
  if (swim === "role" && laneKey !== ALL_LANE) return { role: laneKey };
  if (swim === "agent") return { agent: laneKey };
  if (swim === "package" && laneKey !== NO_PACKAGE) return { pkg: laneKey };
  return {};
}

/**
 * Adds a card from the inline form, then applies the lane's role, agent, or package. An empty
 * title does nothing. The prototype throws for an agent lane it has no model list for; the
 * port keeps the card's model instead.
 */
export function submitQuickAdd(pid: string, col: Column, title: string, extra: LaneExtra): void {
  if (!title.trim()) return;
  batch(() => {
    M.quickAdd(pid, col, title);
    const created = M.S.cards[M.S.cards.length - 1];
    if (!created) return;
    Object.assign(created, extra);
    const model = extra.agent ? M.AGENTS[extra.agent]?.models[0] : undefined;
    if (model) created.model = model;
  });
}

/** Opens the New card dialog on the Bug fix template, starting the card unless it is Backlog. */
export function openTemplateCard(col: Column, extra: LaneExtra): void {
  M.newCard({
    template: "Bug fix",
    start: col !== "backlog",
    ...(extra.role ? { role: extra.role } : {}),
    ...(extra.agent ? { agent: extra.agent } : {}),
  });
}
