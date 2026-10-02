import {
  type IntegrationHistoryItem,
  type IntegrationQueueItem,
  type IntegratorState,
  type MergePhase,
  MergePhaseLanding,
  MergePhaseQueued,
  MergePhaseResolving,
  MergePhaseStopped,
  MergePhaseTesting,
  type IntegrationState as WireIntegrationState,
} from "@marshal/protocol";
import { toMillis } from "./time";

/*
 * What a project's Integrator is doing, in the words the Integration view draws (docs/architecture.md
 * section 8, the merge flow). The daemon answers one read model for the view and the board header:
 * the cards that wait or are being merged, in order, with the phase each is in, and the cards it
 * delivered, newest first. This groups the queue into the view's lanes and says each state in plain
 * words. The cards that stopped and need the owner are not in it: the store already has them.
 */

/** The merge phases in the order a card passes through them, which is the order the lanes draw. */
const LANE_PHASES: readonly MergePhase[] = [
  MergePhaseQueued,
  MergePhaseResolving,
  MergePhaseTesting,
  MergePhaseLanding,
];

/** One card in the merge queue. */
export interface MergeQueueCard {
  /** The daemon's own id for the card, which its routes take. */
  cardId: string;
  /** The card's key, such as "web#12", which is how the store names it. */
  key: string;
  title: string;
  phase: MergePhase;
  /** Counts from 1; the card being merged is 1. */
  position: number;
}

/** One card the Integrator delivered. */
export interface DeliveredCard {
  cardId: string;
  key: string;
  title: string;
  /** When the work landed, in ms. */
  at: number;
  /** The first characters of the commit that landed. */
  commit: string;
  /** The conflicts the Integrator resolved, 0 for a clean merge. */
  resolved: number;
  /** "Resolved 2 conflicts", or empty for a clean merge. */
  resolvedLabel: string;
  /** The Integrator's own report, which may be empty. */
  summary: string;
  /** True while the branch tip is still this merge and the folder is clean. */
  canUndo: boolean;
}

/** A project's merge flow as the Integration view keeps it. */
export interface MergeFlow {
  projectId: string;
  /** The branch finished cards land on, such as "development". */
  target: string;
  /** The Integrator's own branch. */
  integratorBranch: string;
  /** How many commits the Integrator's branch has that the target does not yet. */
  aheadBy: number;
  state: IntegratorState;
  /** The state in plain words: "Idle", "Merging web#12", "Waiting for you", or "Paused". */
  stateLabel: string;
  /** The one sentence that says why the Integrator stopped, or empty. */
  message: string;
  /** The daemon's id for the card being merged, or empty. */
  currentCardId: string;
  /** The queue, grouped by phase. Every phase has a list, so a lane never reads `undefined`. */
  lanes: Record<MergePhase, MergeQueueCard[]>;
  /** How many cards wait or are being merged, in all lanes. */
  queued: number;
  delivered: DeliveredCard[];
  /** The daemon's clock when it answered, in ms. */
  serverTime: number;
}

/**
 * What the store keeps for one project's Integration view: the flow once it was read, the sentence
 * for a read that failed, and whether a first read is still on its way. A daemon with no merge queue
 * is a flow of null and no error, which the view draws as nothing waiting.
 */
export interface MergeFlowSlot {
  flow: MergeFlow | null;
  error: string;
  loading: boolean;
}

/** The first characters of a commit id, which is what a person reads. */
const SHORT_COMMIT = 7;

/** "Resolved 1 conflict" and "Resolved 3 conflicts", or nothing for a merge that had none. */
export function resolvedLabel(count: number): string {
  if (count <= 0) return "";
  return count === 1 ? "Resolved 1 conflict" : `Resolved ${count} conflicts`;
}

const queueCard = (item: IntegrationQueueItem): MergeQueueCard => ({
  cardId: item.cardId,
  key: item.key,
  title: item.title,
  phase: item.phase,
  position: item.position,
});

const deliveredCard = (item: IntegrationHistoryItem): DeliveredCard => ({
  cardId: item.cardId,
  key: item.key,
  title: item.title,
  at: toMillis(item.mergedAt),
  commit: item.commit.slice(0, SHORT_COMMIT),
  resolved: item.resolved,
  resolvedLabel: resolvedLabel(item.resolved),
  summary: item.summary,
  canUndo: item.canUndo,
});

/** The Integrator's state in plain words, naming the card it is merging when there is one. */
function stateLabelOf(state: WireIntegrationState): string {
  switch (state.state) {
    case "merging": {
      const current = state.queue.find((item) => item.cardId === state.currentCardId);
      return current ? `Merging ${current.key}` : "Merging";
    }
    case "waiting":
      return "Waiting for you";
    case "paused":
      return "Paused";
    default:
      return "Idle";
  }
}

/** Groups the queue by phase, keeping the daemon's order inside each lane. */
function lanesOf(queue: readonly IntegrationQueueItem[]): Record<MergePhase, MergeQueueCard[]> {
  const lanes: Record<MergePhase, MergeQueueCard[]> = {
    [MergePhaseQueued]: [],
    [MergePhaseResolving]: [],
    [MergePhaseTesting]: [],
    [MergePhaseLanding]: [],
    [MergePhaseStopped]: [],
  };
  for (const item of queue) lanes[item.phase]?.push(queueCard(item));
  return lanes;
}

/** A project's merge flow from the daemon's answer. */
export function toMergeFlow(state: WireIntegrationState): MergeFlow {
  const lanes = lanesOf(state.queue);
  return {
    projectId: state.projectId,
    target: state.target,
    integratorBranch: state.integratorBranch,
    aheadBy: state.aheadBy,
    state: state.state,
    stateLabel: stateLabelOf(state),
    message: state.message ?? "",
    currentCardId: state.currentCardId ?? "",
    lanes,
    queued: LANE_PHASES.reduce((count, phase) => count + lanes[phase].length, 0),
    delivered: state.history.map(deliveredCard),
    serverTime: toMillis(state.serverTime),
  };
}

/** True when the Integrator is paused, which is when the button says Resume and not Pause. */
export const isPaused = (flow: MergeFlow): boolean => flow.state === "paused";
