import {
  EventTypeCardMoved,
  EventTypeCardUpdated,
  EventTypeMergeProgress,
  type Event as WireEvent,
  type IntegrationState as WireIntegrationState,
} from "@marshal/protocol";
import { createEffect, onCleanup, untrack } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { ApiError } from "~/data/api-error";
import { isRecord } from "~/data/guards";
import { type MergeFlow, type MergeFlowSlot, toMergeFlow } from "~/data/mappers/integration";
import { ChangeInFlightError } from "~/data/optimistic";
import { isDaemon } from "~/data/sections";
import { type Ctx, sectionsOf } from "~/mock/context";
import { toast } from "~/mock/engine";
import { applyCard } from "./cards";

/*
 * The merge flow's data layer: a project's Integrator state (the Integration view), its pause and
 * resume, and a card's retry, undo, and worktree. The daemon owns all of it, so nothing is drawn
 * before it answers and a refusal is shown in its own words by `optimistic`.
 *
 * The state is read when a project's Integration view is shown, and again while it stays shown on
 * every `merge.progress`, `card.moved`, and `card.updated` of that project (all arrive on its topic),
 * on a reconnect, and on a resync. A project whose view is not shown is left alone, so the busy
 * `card.updated` of a running agent costs nothing until someone looks.
 *
 * The three actions the board and the card panel call take only the card and no store, so the store
 * that follows the daemon registers itself here (`followMergeFlow`). They return false with none.
 */

export type WorktreeOpener = "finder" | "editor";

const NOT_CONNECTED = "Marshal is not connected to its daemon.";
const READ_FAILED = "Marshal could not read the merge queue. Try again.";

/** The store these actions act on: set while a store follows the daemon, cleared when it stops. */
let active: Ctx | null = null;
const liveCtx = (ctx: Ctx | null | undefined): Ctx | null => ctx ?? active;

/** Where a project's reads stand: one at a time, and a change that came in meanwhile asks for another. */
interface Reading {
  running: boolean;
  dirty: boolean;
  /** Counts what was written to the project's flow, so an older read cannot overwrite a newer answer. */
  version: number;
}

const readings = new WeakMap<Ctx, Map<string, Reading>>();

function readingOf(ctx: Ctx, projectId: string): Reading {
  let perProject = readings.get(ctx);
  if (!perProject) {
    perProject = new Map();
    readings.set(ctx, perProject);
  }
  let reading = perProject.get(projectId);
  if (!reading) {
    reading = { running: false, dirty: false, version: 0 };
    perProject.set(projectId, reading);
  }
  return reading;
}

function writeSlot(ctx: Ctx, projectId: string, slot: MergeFlowSlot): void {
  ctx.S.integration ??= {};
  ctx.S.integration[projectId] = slot;
}

/** Everything about a flow except the daemon's clock, which changes with every read. */
const signature = (flow: MergeFlow | null): string =>
  flow ? JSON.stringify({ ...flow, serverTime: 0 }) : "";

/**
 * Writes the daemon's answer, whole. An answer that says nothing new is not written, so the lanes do
 * not redraw for a `card.updated` that had nothing to do with the merge.
 */
function applyFlow(ctx: Ctx, wire: WireIntegrationState): void {
  readingOf(ctx, wire.projectId).version += 1;
  const flow = toMergeFlow(wire);
  const before = ctx.S.integration?.[wire.projectId];
  if (before && !before.error && !before.loading && signature(before.flow) === signature(flow)) {
    return;
  }
  writeSlot(ctx, wire.projectId, { flow, error: "", loading: false });
}

/**
 * Keeps what was read when a read fails, with the sentence to show. A daemon with no merge queue, or
 * a project that is gone, answers not_found, which is nothing waiting rather than a failure.
 */
function keepFailure(ctx: Ctx, projectId: string, error: unknown): void {
  if (error instanceof ApiError && error.code === "not_found") {
    writeSlot(ctx, projectId, { flow: null, error: "", loading: false });
    return;
  }
  const message = error instanceof ApiError ? error.message : READ_FAILED;
  const flow = ctx.S.integration?.[projectId]?.flow ?? null;
  writeSlot(ctx, projectId, { flow, error: message, loading: false });
}

async function readOnce(ctx: Ctx, api: ApiClient, projectId: string, reading: Reading) {
  const version = reading.version;
  try {
    const wire = await api.integrationState(projectId);
    // A pause or resume answered while this was on its way: its answer is newer, so read again.
    if (reading.version === version) applyFlow(ctx, wire);
    else reading.dirty = true;
  } catch (error) {
    keepFailure(ctx, projectId, error);
  }
}

/**
 * Reads a project's merge flow. Only one read per project is on its way at a time: a call that comes
 * while one is running asks for one more when it ends, however many came, so a burst of events costs
 * two reads and not a hundred.
 */
async function readMergeFlow(ctx: Ctx, projectId: string): Promise<void> {
  const api = ctx.env.data?.api;
  if (!api) return;
  const reading = readingOf(ctx, projectId);
  if (reading.running) {
    reading.dirty = true;
    return;
  }
  if (!ctx.S.integration?.[projectId]) {
    writeSlot(ctx, projectId, { flow: null, error: "", loading: true });
  }
  reading.running = true;
  try {
    do {
      reading.dirty = false;
      await readOnce(ctx, api, projectId, reading);
    } while (reading.dirty);
  } finally {
    reading.running = false;
  }
}

/** The project whose Integration view is on screen, in the main pane or a split one, or null. */
function shownProject(ctx: Ctx): string | null {
  const { route, split } = ctx.S;
  if (route.page !== "project" || !route.pid) return null;
  return [route.view, ...split].includes("integration") ? route.pid : null;
}

const MERGE_EVENTS: ReadonlySet<string> = new Set([
  EventTypeMergeProgress,
  EventTypeCardMoved,
  EventTypeCardUpdated,
]);

/** The project an event is about: a merge event names it, a card event carries the card that has it. */
function projectOfEvent(data: unknown): string {
  if (!isRecord(data)) return "";
  if (typeof data.projectId === "string") return data.projectId;
  const card = data.card;
  return isRecord(card) && typeof card.projectId === "string" ? card.projectId : "";
}

/** Reads the flow again when an event of the project whose Integration view is shown might change it. */
export function applyMergeFlowEvent(ctx: Ctx, event: WireEvent): void {
  if (!MERGE_EVENTS.has(event.type)) return;
  const projectId = projectOfEvent(event.data);
  if (projectId && projectId === shownProject(ctx)) void readMergeFlow(ctx, projectId);
}

/**
 * Starts reading the merge flow of the project whose Integration view is shown, once the app is
 * ready and again each time the view is shown, and registers the store for the actions below. It
 * answers how to read the shown project again, for a reconnect or a resync; the events are applied
 * by `applyMergeFlowEvent`. It does nothing for a store whose cards are still the mock's.
 */
export function followMergeFlow(ctx: Ctx): () => void {
  if (!ctx.env.data || !isDaemon("S5a", sectionsOf(ctx.env))) return () => undefined;
  active = ctx;
  onCleanup(() => {
    if (active === ctx) active = null;
  });
  createEffect(() => {
    if (!ctx.S.ready) return;
    const projectId = shownProject(ctx);
    if (projectId) untrack(() => void readMergeFlow(ctx, projectId));
  });
  return () => {
    const projectId = shownProject(ctx);
    if (projectId) void readMergeFlow(ctx, projectId);
  };
}

/** Reads a project's flow again, for the Try again that follows a failed read. */
export function reloadMergeFlow(projectId: string, ctx?: Ctx | null): Promise<void> {
  const live = liveCtx(ctx);
  return live ? readMergeFlow(live, projectId) : Promise.resolve();
}

/** The nothing a change draws before the daemon answers: the daemon owns this state. */
const nothing = (): void => undefined;

/**
 * Asks the daemon for one change and hands its answer back, or null when nothing was asked or the
 * daemon said no. A refusal is shown in the daemon's own words by `optimistic`; only the answer for
 * a change that is still running, which is thrown before the daemon is asked, is said here.
 */
async function ask<T>(
  ctx: Ctx | null,
  key: string,
  request: (api: ApiClient) => Promise<T>,
): Promise<T | null> {
  if (!ctx) return null;
  const api = ctx.env.data?.api;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return null;
  }
  try {
    return await ctx.optimistic({
      key,
      apply: nothing,
      request: () => request(api),
      rollback: nothing,
    });
  } catch (error) {
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
    return null;
  }
}

/**
 * The daemon's id for a card. The store knows a card by its key (`web#12`) and the routes take the
 * daemon's own id, so a key is looked up and an id is used as it is. A key the store does not have,
 * and a card the mock made, have no id to ask with.
 */
function daemonIdOf(ctx: Ctx, cardId: string): string | null {
  const stored = ctx.S.cards.find((card) => card.id === cardId);
  if (stored) return stored.daemonId ?? null;
  return cardId.includes("#") ? null : cardId;
}

/** Stops or starts the Integrator taking cards off the queue, and draws the state it answers. */
async function setPaused(
  projectId: string,
  paused: boolean,
  ctx: Ctx | null | undefined,
): Promise<boolean> {
  const live = liveCtx(ctx);
  const state = await ask(live, `merge-flow:${projectId}`, (api) =>
    paused ? api.pauseIntegration(projectId) : api.resumeIntegration(projectId),
  );
  if (!live || !state) return false;
  applyFlow(live, state);
  return true;
}

/** Pauses merging in a project: finished cards wait in the queue until it is resumed. */
export const pauseMerging = (projectId: string, ctx?: Ctx | null): Promise<boolean> =>
  setPaused(projectId, true, ctx);

/** Lets the Integrator take cards off the queue again. */
export const resumeMerging = (projectId: string, ctx?: Ctx | null): Promise<boolean> =>
  setPaused(projectId, false, ctx);

/** Reveals or opens a card's worktree on the machine the daemon runs on. */
export async function openCardWorktree(
  cardId: string,
  opener: WorktreeOpener,
  ctx?: Ctx | null,
): Promise<boolean> {
  const live = liveCtx(ctx);
  const daemonId = live ? daemonIdOf(live, cardId) : null;
  if (!live || !daemonId) return false;
  const opened = await ask(live, `worktree:${daemonId}`, async (api) => {
    await api.openCardWorktree(daemonId, { with: opener });
    return true;
  });
  return opened === true;
}

const MERGE_CALLS = {
  retry: (api: ApiClient, id: string) => api.retryCardMerge(id),
  undo: (api: ApiClient, id: string) => api.undoCardMerge(id),
  send: (api: ApiClient, id: string) => api.sendCardToMerge(id),
};

const MERGE_WORDS = {
  retry: (key: string) => `Retrying the merge of ${key}`,
  undo: (key: string) => `Undid the merge of ${key}`,
  send: (key: string) => `Sent ${key} to the merge queue`,
};

/** Runs a card's merge or delivery again, sends it to the queue, or puts the branch back, and draws the card it answers. */
async function changeMerge(
  cardId: string,
  ctx: Ctx | null | undefined,
  kind: "retry" | "undo" | "send",
): Promise<boolean> {
  const live = liveCtx(ctx);
  const daemonId = live ? daemonIdOf(live, cardId) : null;
  if (!live || !daemonId) return false;
  const card = await ask(live, `merge-${kind}:${daemonId}`, (api) =>
    MERGE_CALLS[kind](api, daemonId),
  );
  if (!card) return false;
  applyCard(live, card);
  toast(live, MERGE_WORDS[kind](card.key));
  if (live.S.integration?.[card.projectId]) void readMergeFlow(live, card.projectId);
  return true;
}

/** Runs a card's merge or delivery again after a stop that needed the owner. */
export const retryMerge = (cardId: string, ctx?: Ctx | null): Promise<boolean> =>
  changeMerge(cardId, ctx, "retry");

/**
 * Sends a card whose work is committed to the merge queue. A project on GitHub is refused with the
 * daemon's own sentence: its cards go through a pull request and a review.
 */
export const sendToMerge = (cardId: string, ctx?: Ctx | null): Promise<boolean> =>
  changeMerge(cardId, ctx, "send");

/** Puts the integration branch back to where it was before a card's merge. */
export const undoMerge = (cardId: string, ctx?: Ctx | null): Promise<boolean> =>
  changeMerge(cardId, ctx, "undo");
