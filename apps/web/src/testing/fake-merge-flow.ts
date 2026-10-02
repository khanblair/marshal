/**
 * The merge-flow routes of the fake daemon (the Integration view): a project's Integrator state, its
 * pause and resume, a card's retry and undo, and showing a card's worktree. The daemon owns the
 * queue and what was delivered, so a test seeds each project's state and the routes change it the
 * way the daemon's own handlers do: pause holds the queue, resume lets it move, a retry puts the card
 * back in the queue, and an undo takes it out of what was delivered and back to Ready to merge.
 *
 * A project with nothing seeded reads as idle with an empty queue. A test that wants the daemon with
 * no merge queue at all (its routes are not registered) sets `absent`, and the queue's routes answer
 * not_found; opening a folder needs only the card and still answers. Opening a folder is only
 * recorded: nothing here starts a program.
 */
import {
  IntegrationBranchName,
  type IntegrationHistoryItem,
  type IntegrationQueueItem,
  type IntegrationState,
  IntegratorStateIdle,
  IntegratorStateMerging,
  IntegratorStatePaused,
  MergePhaseQueued,
  type Card as WireCard,
} from "@marshal/protocol";
import { emptyAnswer, errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { STATUS } from "./fake-card-shared";

/** Sends an event on a topic, as the daemon does after a change. */
type Publish = (topic: string, type: string, data: unknown) => void;

/** Everything the merge-flow routes need to know about the rest of the fake daemon. */
export interface MergeFlowStore {
  /** Every project's state, by project id. A project with no entry reads as idle. */
  states: Record<string, IntegrationState>;
  /** True for a daemon with no merge queue: every route answers not_found. */
  absent: boolean;
  /** The folders a test asked to open, in order. */
  opened: { cardId: string; with: string }[];
  /** The cards the fake daemon holds, which a retry or an undo changes and answers. */
  cards: () => WireCard[];
  /** Whether a project exists. */
  hasProject: (projectId: string) => boolean;
  publish: Publish;
  now: () => string;
}

export interface FakeMergeFlowOptions {
  /** The states it starts with. None by default: every project is idle. */
  states?: readonly IntegrationState[];
  cards: () => WireCard[];
  hasProject: (projectId: string) => boolean;
  publish: Publish;
  now: () => string;
}

export function createMergeFlowStore(options: FakeMergeFlowOptions): MergeFlowStore {
  const states: Record<string, IntegrationState> = {};
  for (const state of options.states ?? []) states[state.projectId] = structuredClone(state);
  return {
    states,
    absent: false,
    opened: [],
    cards: options.cards,
    hasProject: options.hasProject,
    publish: options.publish,
    now: options.now,
  };
}

/** A project that has nothing to merge. */
export function idleFlow(projectId: string, serverTime: string): IntegrationState {
  return {
    projectId,
    target: "main",
    integratorBranch: IntegrationBranchName,
    aheadBy: 0,
    state: IntegratorStateIdle,
    queue: [],
    history: [],
    serverTime,
  };
}

/** One card in the queue. A card is queued at the end unless the test says where it is. */
export function queueItem(
  fields: Partial<IntegrationQueueItem> & Pick<IntegrationQueueItem, "cardId" | "key">,
): IntegrationQueueItem {
  return { title: `Card ${fields.key}`, phase: MergePhaseQueued, position: 1, ...fields };
}

/** One delivered card. A clean merge unless the test says it resolved conflicts. */
export function historyItem(
  fields: Partial<IntegrationHistoryItem> & Pick<IntegrationHistoryItem, "cardId" | "key">,
): IntegrationHistoryItem {
  return {
    title: `Card ${fields.key}`,
    mergedAt: "2026-09-27T09:30:00.000Z",
    commit: "3f2a9c1d8e",
    resolved: 0,
    summary: "",
    canUndo: false,
    ...fields,
  };
}

const NO_MERGE_QUEUE = "Marshal has nothing at that address.";
const NOTHING_TO_UNDO =
  "This card was not delivered by the Integrator, so there is nothing to undo.";

const notFound = () => errorAnswer(STATUS.notFound, "not_found", NO_MERGE_QUEUE);
const noCard = () =>
  errorAnswer(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that card. It may have been removed.",
  );

/** The state with the daemon's own time, so a read counts ages from it. */
function read(store: MergeFlowStore, projectId: string): IntegrationState {
  const state = store.states[projectId] ?? idleFlow(projectId, store.now());
  return { ...structuredClone(state), serverTime: store.now() };
}

/** A project's state to change, made idle the first time. */
function stateOf(store: MergeFlowStore, projectId: string): IntegrationState {
  let state = store.states[projectId];
  if (!state) {
    state = idleFlow(projectId, store.now());
    store.states[projectId] = state;
  }
  return state;
}

/** What the Integrator does after a change: paused stays paused, else it merges while cards wait. */
function settle(state: IntegrationState): void {
  if (state.state === IntegratorStatePaused) return;
  state.state = state.queue.length > 0 ? IntegratorStateMerging : IntegratorStateIdle;
  state.currentCardId = state.queue[0]?.cardId ?? "";
}

function answerState(store: MergeFlowStore, projectId: string, action: string): Response {
  if (!store.hasProject(projectId)) return notFound();
  const state = stateOf(store, projectId);
  if (action === "pause") state.state = IntegratorStatePaused;
  if (action === "resume") {
    state.state = IntegratorStateIdle;
    settle(state);
  }
  return jsonAnswer(read(store, projectId));
}

/** Puts a card back in the queue, as the daemon does for a retry of a stopped merge. */
function retry(store: MergeFlowStore, card: WireCard): Response {
  const state = stateOf(store, card.projectId);
  card.state = "merging";
  card.needsReason = undefined;
  card.mergePhase = MergePhaseQueued;
  state.queue = state.queue.filter((item) => item.cardId !== card.id);
  state.queue.push(
    queueItem({
      cardId: card.id,
      key: card.key,
      title: card.title,
      position: state.queue.length + 1,
    }),
  );
  settle(state);
  store.publish(`project:${card.projectId}`, "merge.progress", {
    projectId: card.projectId,
    cardId: card.id,
    phase: MergePhaseQueued,
  });
  return jsonAnswer(card);
}

/** Takes a delivered card out of what was delivered and back to Ready to merge. */
function undo(store: MergeFlowStore, card: WireCard): Response {
  const state = stateOf(store, card.projectId);
  if (!state.history.some((item) => item.cardId === card.id && item.canUndo)) {
    return errorAnswer(STATUS.refused, "refused", NOTHING_TO_UNDO);
  }
  state.history = state.history.filter((item) => item.cardId !== card.id);
  card.state = "ready";
  store.publish(`project:${card.projectId}`, "card.updated", { card });
  return jsonAnswer(card);
}

/** Answers a merge-flow route, or null when the request is not one. */
export function answerMergeFlowRoute(store: MergeFlowStore, request: FakeRequest): Response | null {
  const path = request.url.replace(/^https?:\/\/[^/]+/, "").split("?")[0] ?? "";
  const project = /^\/v1\/projects\/([^/]+)\/integration(?:\/(pause|resume))?$/.exec(path);
  const onCard = /^\/v1\/cards\/([^/]+)\/(merge\/retry|merge\/undo|worktree\/open)$/.exec(path);
  if (!project && !onCard) return null;
  // Opening a folder needs only the card, not the merge queue, so it answers on every daemon.
  if (store.absent && onCard?.[2] !== "worktree/open") return notFound();
  if (project?.[1]) {
    const reads = request.method === "GET" && !project[2];
    const presses = request.method === "POST" && project[2];
    if (!reads && !presses) return null;
    if (reads && !store.hasProject(project[1])) return notFound();
    return reads
      ? jsonAnswer(read(store, project[1]))
      : answerState(store, project[1], project[2] ?? "");
  }
  if (!onCard || request.method !== "POST") return null;
  const card = store.cards().find((one) => one.id === onCard[1]);
  if (!card) return noCard();
  if (onCard[2] === "merge/retry") return retry(store, card);
  if (onCard[2] === "merge/undo") return undo(store, card);
  const body = request.body ? (JSON.parse(request.body) as { with?: string }) : {};
  store.opened.push({ cardId: card.id, with: body.with ?? "" });
  return emptyAnswer();
}
