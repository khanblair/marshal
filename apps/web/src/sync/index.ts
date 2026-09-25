import { batch, createEffect, createRoot } from "solid-js";
import type { Data } from "~/data";
import { ApiError } from "~/data/api-error";
import { isDaemon } from "~/data/sections";
import { type Ctx, sectionsOf } from "~/mock/context";
import { agentsSyncer } from "./agents";
import { projectsSyncer } from "./projects";
import type { SyncControl } from "./sync-control";
import type { Syncer } from "./syncer";

/**
 * Every section the daemon fills, in the order their snapshots are applied. A section that is
 * still `mock` in `sections.ts` is skipped. The projects (S3) and the agent catalog (S4) are the
 * ones on the daemon so far.
 */
const SYNCERS: readonly Syncer[] = [projectsSyncer, agentsSyncer];

const LOAD_FAILED = "Marshal could not load your data from the daemon. Try again.";

/** The technical side of a failed check, for the collapsed "Details" on the connection screen. */
function describeError(error: ApiError | null): string {
  if (!error) return "";
  const status = error.status === null ? "no answer" : `HTTP ${error.status}`;
  return `${error.code} (${status}): ${error.message}`;
}

interface Progress {
  /** True once the first snapshots are applied. */
  loaded: boolean;
  /** True once the app came online and asked for them. */
  asked: boolean;
}

/**
 * Loads every active section's snapshot and applies them together in one batch, so the app never
 * draws a half-loaded state. Only the newest request is applied: two loads can overlap when a new
 * connection sends both a Resync and a return to online, and an older answer must not win.
 */
function createReloader(ctx: Ctx, data: Data, active: readonly Syncer[], progress: Progress) {
  let newest = 0;
  return async (): Promise<void> => {
    const mine = ++newest;
    try {
      const snapshots = await Promise.all(active.map((syncer) => syncer.load(data.api)));
      if (mine !== newest) return;
      batch(() => {
        active.forEach((syncer, i) => {
          syncer.apply(ctx, snapshots[i]);
        });
        progress.loaded = true;
        ctx.S.loadError = "";
        ctx.S.ready = true;
      });
    } catch (error) {
      if (mine !== newest) return;
      ctx.S.loadError = error instanceof ApiError ? error.message : LOAD_FAILED;
    }
  };
}

/**
 * Follows the connection into `S.connection` and `S.ready`. `ready` means the app frame can draw:
 * the first snapshots are in, or the connection is in a state whose full screen must draw instead.
 * The first time the app is online it loads the snapshots; after that the machine's own
 * `onReconnected` and the stream's `Resync` do, so nothing loads a third time.
 */
function followConnection(ctx: Ctx, data: Data, progress: Progress, reload: () => Promise<void>) {
  const { connection, tokens } = data;
  createEffect(() => {
    const state = connection.state();
    const error = connection.lastError();
    const retryAt = connection.retryAt();
    const refused = state === "unauthorized" && tokens.get() !== null;
    batch(() => {
      ctx.S.connection = {
        state,
        retryAt,
        detail: describeError(error),
        rejection: refused ? (error?.message ?? "") : "",
        busy: false,
      };
      ctx.S.ready = progress.loaded || state === "unreachable" || state === "unauthorized";
    });
    if (state === "online" && !progress.asked) {
      progress.asked = true;
      void reload();
    }
  });
}

/**
 * Connects the store to the daemon: it starts the connection, follows its state, loads the
 * snapshots of every daemon-backed section, keeps them current from the event stream, and loads
 * them again after a return or a `Resync`. It does nothing (and returns null) in a store with no
 * `data`, which is how a unit test that needs no daemon runs.
 */
export function startSync(ctx: Ctx, syncers: readonly Syncer[] = SYNCERS): SyncControl | null {
  const { data } = ctx.env;
  if (!data) return null;
  const table = sectionsOf(ctx.env);
  const active = syncers.filter((syncer) => isDaemon(syncer.section, table));
  const progress: Progress = { loaded: false, asked: false };
  const reload = createReloader(ctx, data, active, progress);
  const stops = [
    data.onEvents((events) =>
      batch(() => {
        for (const event of events) for (const syncer of active) syncer.onEvent?.(ctx, event);
      }),
    ),
    data.onResync(() => void reload()),
    data.connection.onReconnected(() => void reload()),
  ];
  const disposeEffects = createRoot((dispose) => {
    followConnection(ctx, data, progress, reload);
    return dispose;
  });
  data.stream.subscribe([...new Set(active.flatMap((syncer) => syncer.topics))]);
  data.start();
  const control: SyncControl = {
    reload,
    stop() {
      for (const stop of stops) stop();
      disposeEffects();
      data.stop();
      ctx.sync = null;
    },
  };
  ctx.sync = control;
  return control;
}
