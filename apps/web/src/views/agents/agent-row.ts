import { type Card, M } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { type AgentAction, actionsFor, openChatAction } from "./agent-actions";
import { activityOf, sessionIcon, sessionLabel } from "./agents-model";

/** Everything the table row and the phone row of one session show. */
export interface AgentRowData {
  /** Value of `data-card`: the card key, or empty for the Orchestrator. */
  dataCard: CardKey | "";
  num: string;
  title: string;
  bypass: boolean;
  role: string;
  agent: string;
  model: string;
  think: string;
  perm: string;
  /** Status key that gives the State column its flag and color. */
  state: string;
  stateLabel: string;
  sess: string;
  sessIcon: string;
  pinned: boolean;
  /** Show the pulsing working dot before the activity. */
  pulse: boolean;
  activity: string;
  /** Text color class of the activity, empty for the default. */
  activityClass: string;
  cost: string;
  aria: string;
  actions: AgentAction[];
}

const ORCHESTRATOR_COST_USD = 0.42;

/** The project's Orchestrator session, which always heads the table. */
export function orchestratorRow(projectName: string): AgentRowData {
  return {
    dataCard: "",
    num: "",
    title: `Orchestrator for ${projectName}`,
    bypass: false,
    role: "Orchestrator",
    agent: "Claude Code",
    model: "claude-opus-4-1",
    think: "High",
    perm: "Plan only",
    state: "planning",
    stateLabel: "Awake",
    sess: "Awake",
    sessIcon: "sun",
    pinned: false,
    pulse: false,
    activity: "Watching the board",
    activityClass: "text-secondary",
    cost: M.money(ORCHESTRATOR_COST_USD),
    aria: "Orchestrator session",
    actions: [openChatAction()],
  };
}

/** The row of one card session. Read it inside a memo so it follows the card. */
export function agentRowOf(card: Card): AgentRowData {
  const d = M.deco(card);
  return {
    dataCard: card.id,
    num: d.num,
    title: card.title,
    bypass: card.bypass,
    role: card.role,
    agent: card.agent,
    model: card.model,
    think: d.think || "Not supported",
    perm: card.perm,
    state: card.state,
    stateLabel: d.stateLabel,
    sess: sessionLabel(card),
    sessIcon: sessionIcon(card),
    pinned: card.pinned,
    pulse: d.showDoing && card.state === "working",
    activity: activityOf(card),
    activityClass: card.state === "needs" ? "text-status-needs-you-text" : "",
    cost: d.cost,
    aria: d.aria,
    actions: actionsFor(card),
  };
}
