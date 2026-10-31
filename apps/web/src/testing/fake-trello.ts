/**
 * The Trello apply routes of the fake daemon: a project's pending changes, and the apply, dismiss,
 * and refresh that change them. The daemon owns the list, so a test seeds each project's state and
 * the routes change it the way the daemon's own handlers do: an apply writes the chosen changes
 * (a conflict only when it is also overridden) and keeps the rest, a dismiss hides, and a refresh
 * sets the named cards back to Trello's title, description, and ticks. Every change publishes
 * `trello.changed` on the project's topic, as the daemon does.
 *
 * A project with nothing seeded is not linked to a Trello board. Nothing here talks to Trello: what
 * "was written" is only recorded in `written`.
 */
import {
  type ApplyTrelloRequest,
  type ApplyTrelloResult,
  type DismissTrelloRequest,
  EventTypeTrelloChanged,
  type RefreshTrelloRequest,
  type RefreshTrelloResult,
  type TrelloApplySkip,
  type TrelloChange,
  type TrelloLinkedCard,
  type TrelloPending,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { STATUS } from "./fake-card-shared";

type Publish = (topic: string, type: string, data: unknown) => void;

/** Everything the Trello routes need to know about the rest of the fake daemon. */
export interface TrelloStore {
  /** Every project's state, by project id. A project with no entry is not linked. */
  states: Record<string, TrelloPending>;
  /** The change ids written to Trello, in order. */
  written: string[];
  /** The card ids a refresh set back to Trello's version, in order. */
  kept: string[];
  /** Why a change cannot be written, by change id: it is skipped with this sentence. */
  refused: Record<string, string>;
  /** How many cards the next refresh makes from Trello cards Marshal had not seen. */
  imports: number;
  hasProject: (projectId: string) => boolean;
  publish: Publish;
  now: () => string;
}

export interface FakeTrelloOptions {
  /** The states it starts with, one per linked project. None by default: no project is linked. */
  states?: Readonly<Record<string, TrelloPending>>;
  hasProject: (projectId: string) => boolean;
  publish: Publish;
  now: () => string;
}

export function createTrelloStore(options: FakeTrelloOptions): TrelloStore {
  return {
    states: structuredClone({ ...options.states }),
    written: [],
    kept: [],
    refused: {},
    imports: 0,
    hasProject: options.hasProject,
    publish: options.publish,
    now: options.now,
  };
}

/** One pending change. A calm comment by the agent unless the test says otherwise. */
export function trelloChange(
  fields: Partial<TrelloChange> & Pick<TrelloChange, "id" | "cardId">,
): TrelloChange {
  return {
    cardTitle: "A card",
    kind: "addComment",
    summary: "Add a comment",
    detail: "",
    byKind: "agent",
    conflict: false,
    conflictNote: "",
    ...fields,
  };
}

/** A project linked to a board, with these changes and these Trello cards. */
export function linkedTrello(
  changes: readonly TrelloChange[],
  cards: readonly TrelloLinkedCard[] = [],
  serverTime = "2026-09-27T09:30:00.000Z",
): TrelloPending {
  return {
    linked: true,
    boardName: "Mobile app",
    changes: [...changes],
    cards: [...cards],
    fetchedAt: "2026-09-27T09:26:00.000Z",
    serverTime,
  };
}

const CONFLICT_SKIPPED = "Trello changed this after Marshal last read it, so it was left alone.";
const NO_LONGER_PENDING = "This change is no longer pending.";

/** The kinds a refresh sets back to Trello's version for a card: its title, description, and ticks. */
const RESET_KINDS: ReadonlySet<string> = new Set(["updateCard", "setItem"]);

const noProject = () =>
  errorAnswer(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that project. It may have been removed.",
  );

const unlinked = (serverTime: string): TrelloPending => ({
  linked: false,
  boardName: "",
  changes: [],
  cards: [],
  fetchedAt: null,
  serverTime,
});

/** The state with the daemon's own time, so a read counts ages from it. */
function read(store: TrelloStore, projectId: string): TrelloPending {
  const state = store.states[projectId] ?? unlinked(store.now());
  return { ...structuredClone(state), serverTime: store.now() };
}

function announce(store: TrelloStore, projectId: string): void {
  store.publish(`project:${projectId}`, EventTypeTrelloChanged, { projectId });
}

/** Answers what the daemon answers for a change that is not written, or null when it is. */
function whySkipped(
  store: TrelloStore,
  state: TrelloPending,
  id: string,
  overrides: readonly string[],
): string | null {
  const change = state.changes.find((one) => one.id === id);
  if (!change) return NO_LONGER_PENDING;
  const refused = store.refused[id];
  if (refused) return refused;
  return change.conflict && !overrides.includes(id) ? CONFLICT_SKIPPED : null;
}

function apply(store: TrelloStore, projectId: string, body: ApplyTrelloRequest): Response {
  const state = store.states[projectId];
  if (!state) return jsonAnswer({ applied: [], skipped: [], pending: read(store, projectId) });
  const applied: string[] = [];
  const skipped: TrelloApplySkip[] = [];
  for (const id of body.changeIds) {
    const reason = whySkipped(store, state, id, body.overrideIds);
    if (reason === null) applied.push(id);
    else skipped.push({ changeId: id, reason });
  }
  state.changes = state.changes.filter((change) => !applied.includes(change.id));
  store.written.push(...applied);
  if (applied.length > 0) announce(store, projectId);
  const result: ApplyTrelloResult = { applied, skipped, pending: read(store, projectId) };
  return jsonAnswer(result);
}

function dismiss(store: TrelloStore, projectId: string, body: DismissTrelloRequest): Response {
  const state = store.states[projectId];
  if (state) {
    state.changes = state.changes.filter((change) => !body.changeIds.includes(change.id));
    announce(store, projectId);
  }
  return jsonAnswer(read(store, projectId));
}

function refresh(store: TrelloStore, projectId: string, body: RefreshTrelloRequest): Response {
  const state = store.states[projectId];
  if (state) {
    const before = state.changes.length;
    state.changes = state.changes.filter(
      (change) => !(body.keepTrelloCardIds.includes(change.cardId) && RESET_KINDS.has(change.kind)),
    );
    store.kept.push(...body.keepTrelloCardIds);
    state.fetchedAt = store.now();
    announce(store, projectId);
    const result: RefreshTrelloResult = {
      imported: store.imports,
      updated: before - state.changes.length,
      pending: read(store, projectId),
    };
    return jsonAnswer(result);
  }
  const result: RefreshTrelloResult = { imported: 0, updated: 0, pending: read(store, projectId) };
  return jsonAnswer(result);
}

/** Answers a Trello route, or null when the request is not one. */
export function answerTrelloRoute(store: TrelloStore, request: FakeRequest): Response | null {
  const path = request.url.replace(/^https?:\/\/[^/]+/, "").split("?")[0] ?? "";
  const match = /^\/v1\/projects\/([^/]+)\/trello\/(pending|apply|dismiss|refresh)$/.exec(path);
  const projectId = match?.[1] ? decodeURIComponent(match[1]) : "";
  const action = match?.[2];
  if (!(projectId && action)) return null;
  const reads = request.method === "GET" && action === "pending";
  const writes = request.method === "POST" && action !== "pending";
  if (!reads && !writes) return null;
  if (!store.hasProject(projectId)) return noProject();
  if (reads) return jsonAnswer(read(store, projectId));
  const body = request.body ? (JSON.parse(request.body) as Record<string, string[]>) : {};
  if (action === "apply") return apply(store, projectId, body as unknown as ApplyTrelloRequest);
  if (action === "dismiss") {
    return dismiss(store, projectId, body as unknown as DismissTrelloRequest);
  }
  return refresh(store, projectId, body as unknown as RefreshTrelloRequest);
}
