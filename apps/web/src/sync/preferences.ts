import {
  EventTypeMeUpdated,
  EventTypeProjectRemoved,
  MeTopic,
  type Preferences,
  type UpdatePreferencesRequest,
  type Event as WireEvent,
} from "@marshal/protocol";
import { createEffect, untrack } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { ApiError } from "~/data/api-error";
import { isRecord } from "~/data/guards";
import type { Ctx } from "~/mock/context";
import { type PrefWrite, writePrefs } from "./prefs-apply";
import {
  changedKeys,
  completeBase,
  defaultBase,
  dropProjectKeys,
  type PrefMap,
  prefsFromWire,
  projectKey,
  readPrefs,
  samePref,
  toRequest,
} from "./prefs-model";
import type { Syncer } from "./syncer";

/**
 * Section S32: the theme, the List's columns, both tables' sort, and each project's last view,
 * filters, search, swimlane, folded lanes, and saved view in use (decision D2). They follow the
 * person between devices, so they load with the person, arrive as `me.updated`, and are saved
 * whenever they change.
 *
 * The screens write these straight into `M.S`, so saving does not wait for an action. An effect
 * reads them all, compares them with what the daemon last agreed to (`base`, see `prefs-model.ts`),
 * and after a short wait sends only what differs. A search is saved after a longer one, so typing
 * is never one request per key. Only one save is in flight at a time, so the daemon applies them in
 * the order they were made.
 */

/** How long a change waits for others to join it in one request. */
const BATCH_MS = 150;
/** How long typed search text waits: it is saved when typing pauses, not on every key. */
const QUERY_MS = 700;

interface PrefsState {
  /** What the daemon holds, as far as this device knows: the last answer, and what it accepted since. */
  base: PrefMap;
  /** True once the daemon's preferences are in the store. Nothing is compared before, so nothing is sent first. */
  loaded: boolean;
  api: ApiClient | null;
  timer: ReturnType<typeof setTimeout> | null;
  writing: boolean;
  /** True after a save could not reach the daemon. The changes wait for the next load. */
  offline: boolean;
  /**
   * The request the daemon last refused, as text. The same request is not sent again: a value the
   * store could not be put back from (or a refusal that would repeat) must not become a loop of
   * toasts. Any other change makes a different request, which is sent.
   */
  refused: string | null;
}

const states = new WeakMap<Ctx, PrefsState>();

function stateOf(ctx: Ctx): PrefsState {
  let state = states.get(ctx);
  if (!state) {
    state = {
      base: untrack(() => defaultBase(ctx.S)),
      loaded: false,
      api: null,
      timer: null,
      writing: false,
      offline: false,
      refused: null,
    };
    states.set(ctx, state);
  }
  return state;
}

/** Applies what the daemon says, key by key: a value the person changed and the daemon has not seen stays. */
function absorb(ctx: Ctx, state: PrefsState, remote: PrefMap): void {
  const { S } = ctx;
  completeBase(state.base, S.projects);
  const now = untrack(() => readPrefs(S));
  const writes: PrefWrite[] = [];
  for (const [key, value] of remote) {
    const unchanged = samePref(now.get(key), state.base.get(key));
    if (unchanged && !samePref(now.get(key), value)) writes.push([key, value]);
    state.base.set(key, value);
  }
  writePrefs(ctx, writes);
}

/** A change that reaches no daemon: it is kept and sent when the connection is back. */
const isOffline = (error: unknown): boolean =>
  error instanceof ApiError &&
  (error.code === "unreachable" || error.code === "timeout" || error.code === "unavailable");

const OFFLINE: unique symbol = Symbol("offline");

/** Asks the daemon to save, and answers `OFFLINE` instead of failing when it cannot be reached. */
async function saveOrOffline(
  api: ApiClient,
  request: UpdatePreferencesRequest,
): Promise<Preferences | typeof OFFLINE> {
  try {
    return await api.updatePreferences(request);
  } catch (error) {
    if (isOffline(error)) return OFFLINE;
    throw error;
  }
}

/** Puts back the values the daemon refused, unless the person has changed them again since. */
function rollBack(ctx: Ctx, state: PrefsState, sent: ReadonlyMap<string, unknown>): void {
  const now = untrack(() => readPrefs(ctx.S));
  const writes: PrefWrite[] = [];
  for (const [key, value] of sent) {
    const was = state.base.get(key);
    if (was !== undefined && samePref(now.get(key), value)) writes.push([key, was]);
  }
  writePrefs(ctx, writes);
}

/** Sends what differs, once. It returns after the answer, and sends again if more changed meanwhile. */
async function flush(ctx: Ctx, state: PrefsState): Promise<void> {
  state.timer = null;
  const { api } = state;
  if (!api || !state.loaded || state.writing || state.offline) return;
  completeBase(state.base, ctx.S.projects);
  const now = untrack(() => readPrefs(ctx.S));
  const keys = changedKeys(state.base, now);
  if (keys.length === 0) return;
  const sent = new Map(keys.map((key) => [key, now.get(key)]));
  const request = toRequest(now, keys);
  if (JSON.stringify(request) === state.refused) return;
  state.writing = true;
  try {
    const answer = await ctx.optimistic({
      apply: () => undefined,
      request: () => saveOrOffline(api, request),
      rollback: () => rollBack(ctx, state, sent),
    });
    if (answer === OFFLINE) state.offline = true;
    else {
      state.refused = null;
      // The daemon took what was sent, whatever it keeps: a project whose values are its defaults
      // is not kept at all, and must not be sent again for that.
      for (const [key, value] of sent) state.base.set(key, value);
      absorb(ctx, state, prefsFromWire(answer, ctx.S));
    }
  } catch {
    // Refused: `optimistic` showed the daemon's sentence and rolled the store back.
    state.refused = JSON.stringify(request);
  } finally {
    state.writing = false;
  }
  review(ctx, state);
}

/** Waits, then sends, when something differs from what the daemon has. Another change restarts the wait. */
function review(ctx: Ctx, state: PrefsState, seen?: PrefMap): void {
  if (!state.loaded || state.offline) return;
  completeBase(state.base, ctx.S.projects);
  const now = seen ?? untrack(() => readPrefs(ctx.S));
  const keys = changedKeys(state.base, now);
  if (state.timer) clearTimeout(state.timer);
  state.timer = null;
  // Nothing differs any more (the refused change was put back), so the same change may be tried again.
  if (keys.length === 0) state.refused = null;
  if (keys.length === 0 || state.writing) return;
  if (JSON.stringify(toRequest(now, keys)) === state.refused) return;
  const onlySearch = keys.every((key) => key.endsWith(":query"));
  state.timer = setTimeout(() => void flush(ctx, state), onlySearch ? QUERY_MS : BATCH_MS);
}

/** Takes the daemon's preferences into the store, and sends what the person changed while it was away. */
function applyPreferences(ctx: Ctx, wire: Preferences): void {
  const state = stateOf(ctx);
  absorb(ctx, state, prefsFromWire(wire, ctx.S));
  state.loaded = true;
  state.offline = false;
  review(ctx, state);
}

/** The daemon's `me.updated` carries the preferences as they are now, so applying it twice changes nothing. */
function applyMeEvent(ctx: Ctx, event: WireEvent): void {
  if (
    event.type === EventTypeMeUpdated &&
    isRecord(event.data) &&
    isRecord(event.data.preferences)
  ) {
    applyPreferences(ctx, event.data.preferences as unknown as Preferences);
    return;
  }
  // Removing a project removes its preferences with it, and no `me.updated` says so.
  if (event.type === EventTypeProjectRemoved && isRecord(event.data)) {
    const pid = event.data.projectId;
    if (typeof pid === "string") forgetProjectPrefs(ctx, pid);
  }
}

/**
 * The project is gone from the daemon, and so are its preferences. Its screen state goes too, so a
 * project added again under the same id starts as any new project does, as it does on the daemon.
 */
export function forgetProjectPrefs(ctx: Ctx, pid: string): void {
  const { S } = ctx;
  dropProjectKeys(stateOf(ctx).base, pid);
  for (const state of [S.filters, S.query, S.swim, S.lastView, S.showAllDone, S.savedView]) {
    delete state[pid];
  }
  for (const key of Object.keys(S.laneCollapsed)) {
    if (key.startsWith(`${pid}:`)) delete S.laneCollapsed[key];
  }
}

/**
 * A saved view in use was deleted, so the daemon already let it go without saying so. The base
 * follows, or the store's change to none would be sent as if the person had made it.
 */
export function noteSavedViewGone(ctx: Ctx, pid: string): void {
  const state = states.get(ctx);
  if (state?.base.has(projectKey(pid, "savedViewId"))) {
    state.base.set(projectKey(pid, "savedViewId"), null);
  }
}

/** Watches the preferences in the store and saves them. It returns how to stop. */
function startPreferences(ctx: Ctx, api: ApiClient): () => void {
  const state = stateOf(ctx);
  state.api = api;
  createEffect(() => {
    // Reading every preference is what makes this run again for each change to any of them.
    const now = readPrefs(ctx.S);
    untrack(() => review(ctx, state, now));
  });
  return () => {
    if (state.timer) clearTimeout(state.timer);
    state.timer = null;
    states.delete(ctx);
  };
}

export const preferencesSyncer: Syncer<Preferences> = {
  section: "S32",
  topics: [MeTopic],
  load: (api) => api.preferences(),
  apply: applyPreferences,
  onEvent: applyMeEvent,
  start: startPreferences,
};
