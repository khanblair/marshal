import {
  type ApplyTrelloRequest,
  EventTypeTrelloChanged,
  type TrelloPending,
  type Event as WireEvent,
} from "@marshal/protocol";
import { createEffect, untrack } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { ApiError } from "~/data/api-error";
import { isRecord } from "~/data/guards";
import {
  type TrelloApplyReport,
  type TrelloSlot,
  type TrelloView,
  toApplyReport,
  toTrelloView,
} from "~/data/mappers/trello";
import { ChangeInFlightError } from "~/data/optimistic";
import { isDaemon } from "~/data/sections";
import { type Ctx, sectionsOf } from "~/mock/context";
import { toast } from "~/mock/engine";
import { mockApply, mockDismiss, mockRefresh } from "~/mock/trello";

/*
 * The Trello apply flow's data layer (docs/marshal-product-scope.md section 19.2). Marshal works on
 * its own copy of a linked board, so the daemon owns the list of pending changes and nothing is
 * drawn before it answers. The list is read whenever a project is shown, and again on every
 * `trello.changed` of that project, on a reconnect, and on a resync. Every action answers the new
 * list, which is written as it arrives, so a screen never waits for a second read.
 *
 * A store with no daemon (a test, or the mock) has nothing to read: the list is whatever is in
 * `S.trello`, and the actions work on it (`mock/trello.ts`).
 */

/** The events the daemon sends on a project's topic when its pending changes or links change. */
const TRELLO_EVENTS: ReadonlySet<string> = new Set([EventTypeTrelloChanged]);

const NOT_CONNECTED = "Marshal is not connected to its daemon.";
const NOT_LINKED = "This project is not linked to a Trello board.";
const READ_FAILED = "Marshal could not read the Trello changes. Try again.";

/** Where a project's reads stand: one at a time, and a change that came in meanwhile asks for another. */
interface Reading {
  running: boolean;
  dirty: boolean;
  /** Counts what was written to the project's slot, so an older read cannot overwrite a newer answer. */
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

function writeSlot(ctx: Ctx, projectId: string, slot: TrelloSlot): void {
  ctx.S.trello ??= {};
  ctx.S.trello[projectId] = slot;
}

/** Everything about a state except the daemon's clock, which changes with every read. */
const signature = (view: TrelloView | null): string =>
  view ? JSON.stringify({ ...view, serverTime: 0 }) : "";

/** Writes a state, whole. One that says nothing new is not written, so nothing redraws for it. */
function writeView(ctx: Ctx, projectId: string, view: TrelloView): void {
  readingOf(ctx, projectId).version += 1;
  const before = ctx.S.trello?.[projectId];
  if (before && !before.error && !before.loading && signature(before.pending) === signature(view)) {
    return;
  }
  writeSlot(ctx, projectId, { pending: view, error: "", loading: false });
}

const writePending = (ctx: Ctx, projectId: string, wire: TrelloPending): void =>
  writeView(ctx, projectId, toTrelloView(wire));

/** Keeps what was read when a read fails, with the sentence to show. */
function keepFailure(ctx: Ctx, projectId: string, error: unknown): void {
  const message = error instanceof ApiError ? error.message : READ_FAILED;
  const pending = ctx.S.trello?.[projectId]?.pending ?? null;
  writeSlot(ctx, projectId, { pending, error: message, loading: false });
}

async function readOnce(ctx: Ctx, api: ApiClient, projectId: string, reading: Reading) {
  const version = reading.version;
  try {
    const wire = await api.trelloPending(projectId);
    // An action answered while this was on its way: its answer is newer, so read again.
    if (reading.version === version) writePending(ctx, projectId, wire);
    else reading.dirty = true;
  } catch (error) {
    keepFailure(ctx, projectId, error);
  }
}

/**
 * Reads a project's pending changes. Only one read per project is on its way at a time: a call that
 * comes while one is running asks for one more when it ends, however many came.
 */
async function readTrello(ctx: Ctx, projectId: string): Promise<void> {
  const api = ctx.env.data?.api;
  if (!api) return;
  const reading = readingOf(ctx, projectId);
  if (reading.running) {
    reading.dirty = true;
    return;
  }
  if (!ctx.S.trello?.[projectId]) {
    writeSlot(ctx, projectId, { pending: null, error: "", loading: true });
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

/** The project that is on screen, or null. Its cards, its board, and its card panel all read from it. */
function shownProject(ctx: Ctx): string | null {
  const { route } = ctx.S;
  return route.page === "project" && route.pid ? route.pid : null;
}

/** The project an event is about: its payload names it, and its topic is `project:<id>`. */
function projectOfEvent(event: WireEvent): string {
  const { data } = event;
  if (isRecord(data) && typeof data.projectId === "string") return data.projectId;
  return event.topic.startsWith("project:") ? event.topic.slice("project:".length) : "";
}

/**
 * Reads the changes again when a `trello.changed` names a project that is on screen or already
 * read. A project that was never shown has nothing to keep current.
 */
export function applyTrelloEvent(ctx: Ctx, event: WireEvent): void {
  if (!TRELLO_EVENTS.has(event.type)) return;
  const projectId = projectOfEvent(event);
  if (projectId && (projectId === shownProject(ctx) || ctx.S.trello?.[projectId])) {
    void readTrello(ctx, projectId);
  }
}

/**
 * Starts reading the pending changes of the project that is shown, once the app is ready and again
 * each time another project is shown. It answers how to read the shown project again, for a
 * reconnect or a resync. It does nothing for a store with no daemon.
 */
export function followTrello(ctx: Ctx): () => void {
  if (!ctx.env.data || !isDaemon("S5a", sectionsOf(ctx.env))) return () => undefined;
  createEffect(() => {
    if (!ctx.S.ready) return;
    const projectId = shownProject(ctx);
    if (projectId) untrack(() => void readTrello(ctx, projectId));
  });
  return () => {
    const projectId = shownProject(ctx);
    if (projectId) void readTrello(ctx, projectId);
  };
}

/** Reads a project's changes again, for the Try again that follows a failed read. */
export const reloadTrello = (ctx: Ctx, projectId: string): Promise<void> =>
  readTrello(ctx, projectId);

/** The nothing a change draws before the daemon answers: the daemon owns this state. */
const nothing = (): void => undefined;

/**
 * Asks the daemon for one change and hands its answer back, or null when it said no. A refusal is
 * shown in the daemon's own words by `optimistic`; only a change that is still running, which is
 * thrown before the daemon is asked, is said here.
 */
async function ask<T>(
  ctx: Ctx,
  api: ApiClient,
  projectId: string,
  request: (api: ApiClient) => Promise<T>,
): Promise<T | null> {
  try {
    return await ctx.optimistic({
      key: `trello:${projectId}`,
      apply: nothing,
      request: () => request(api),
      rollback: nothing,
    });
  } catch (error) {
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
    return null;
  }
}

/** The state a store with no daemon works on, or null with the reason said as a toast. */
function localView(ctx: Ctx, projectId: string): TrelloView | null {
  const view = ctx.S.trello?.[projectId]?.pending;
  if (!view?.linked) {
    toast(ctx, view ? NOT_LINKED : NOT_CONNECTED);
    return null;
  }
  return view;
}

/**
 * Writes the chosen changes to Trello and answers what was written and what was left, with the
 * reason, in the words of the changes the person saw. The state that follows is drawn as it arrives.
 */
export async function applyTrelloChanges(
  ctx: Ctx,
  projectId: string,
  request: ApplyTrelloRequest,
): Promise<TrelloApplyReport | null> {
  const before = ctx.S.trello?.[projectId]?.pending?.changes ?? [];
  const api = ctx.env.data?.api;
  if (!api) {
    const view = localView(ctx, projectId);
    if (!view) return null;
    const done = mockApply(view, request.changeIds, request.overrideIds);
    writeView(ctx, projectId, done.next);
    return toApplyReport(before, done);
  }
  const result = await ask(ctx, api, projectId, (one) => one.applyTrello(projectId, request));
  if (!result) return null;
  writePending(ctx, projectId, result.pending);
  return toApplyReport(before, result);
}

/** Stops showing the chosen changes. They stay in Marshal and are never written to Trello. */
export async function dismissTrelloChanges(
  ctx: Ctx,
  projectId: string,
  changeIds: readonly string[],
): Promise<boolean> {
  const api = ctx.env.data?.api;
  if (!api) {
    const view = localView(ctx, projectId);
    if (view) writeView(ctx, projectId, mockDismiss(view, changeIds));
    return view !== null;
  }
  const wire = await ask(ctx, api, projectId, (one) =>
    one.dismissTrello(projectId, { changeIds: [...changeIds] }),
  );
  if (!wire) return false;
  writePending(ctx, projectId, wire);
  return true;
}

/** What a refresh did, in counts. */
export interface TrelloRefresh {
  imported: number;
  updated: number;
}

/**
 * Reads the board again. The cards named take Trello's title, description, and ticks, and lose their
 * pending changes of those kinds. With none named, nothing Marshal edited changes.
 */
export async function refreshFromTrello(
  ctx: Ctx,
  projectId: string,
  keepTrelloCardIds: readonly string[] = [],
): Promise<TrelloRefresh | null> {
  const api = ctx.env.data?.api;
  if (!api) {
    const view = localView(ctx, projectId);
    if (!view) return null;
    writeView(ctx, projectId, mockRefresh(view, keepTrelloCardIds));
    return { imported: 0, updated: keepTrelloCardIds.length };
  }
  const result = await ask(ctx, api, projectId, (one) =>
    one.refreshTrello(projectId, { keepTrelloCardIds: [...keepTrelloCardIds] }),
  );
  if (!result) return null;
  writePending(ctx, projectId, result.pending);
  return { imported: result.imported, updated: result.updated };
}

/** The sentence for a refresh the person asked for, as a toast. */
export function refreshWords(result: TrelloRefresh): string {
  if (result.imported === 0 && result.updated === 0) return "Read Trello. Nothing is new.";
  const parts: string[] = [];
  if (result.imported > 0)
    parts.push(`${result.imported} new ${result.imported === 1 ? "card" : "cards"}`);
  if (result.updated > 0)
    parts.push(`${result.updated} ${result.updated === 1 ? "card" : "cards"} updated`);
  return `Read Trello. ${parts.join(", ")}.`;
}

/** Opens the Apply to Trello dialog for a project, with its list read again so it is current. */
export function openTrelloApply(ctx: Ctx, projectId: string): void {
  ctx.S.trelloApply = { pid: projectId };
  void readTrello(ctx, projectId);
}

/** Closes the Apply to Trello dialog. */
export function closeTrelloApply(ctx: Ctx): void {
  ctx.S.trelloApply = null;
}
