import { batch, createEffect, createRoot } from "solid-js";
import type { Data } from "~/data";
import { ApiError } from "~/data/api-error";
import { isDaemon } from "~/data/sections";
import { type Ctx, sectionsOf } from "~/mock/context";
import { agentsSyncer } from "./agents";
import { applyCardSessionEvent, followOpenCard } from "./card-session";
import { applyTerminalFrame, applyTerminalOutputEvent, followOpenCardTerminal } from "./card-view";
import { cardsSyncer } from "./cards";
import { applyChatSessionEvent, followOpenChat } from "./chat-session";
import { chatsSyncer } from "./chats";
import { homeFeedSyncer } from "./home-feed";
import { homeStatsSyncer } from "./home-stats";
import { preferencesSyncer } from "./preferences";
import { profileSyncer } from "./profile";
import { progressSyncer } from "./progress";
import { projectsSyncer } from "./projects";
import { savedViewsSyncer } from "./saved-views";
import type { SyncControl } from "./sync-control";
import type { Syncer } from "./syncer";

/**
 * Every section the daemon fills, in the order their snapshots are applied. A section that is
 * still `mock` in `sections.ts` is skipped. The projects (S3), the agent catalog (S4), and the
 * boards (S5a) are the ones on the daemon so far; the project chats (S17), the Home numbers (S19a),
 * and the Home activity stream (S20) are built and join this list the moment their section is
 * switched, which is a change to `sections.ts` alone.
 *
 * The person's own sections come last. The saved views (S6a) are read per project, so they need the
 * projects; the preferences (S32) name the saved view in use by id, so they need the saved views.
 */
const SYNCERS: readonly Syncer[] = [
  projectsSyncer,
  agentsSyncer,
  cardsSyncer,
  chatsSyncer,
  homeStatsSyncer,
  homeFeedSyncer,
  savedViewsSyncer,
  preferencesSyncer,
  profileSyncer,
  progressSyncer,
];

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
 * Loads every active section's snapshot and applies them, in the order the sections are listed.
 * Each snapshot is applied before the next section is loaded, because one section's snapshot can
 * name what the next one needs: a board is one answer per project, so the projects must be in the
 * store before the boards are asked for. Nothing is drawn until `ready` is set, so the app still
 * never shows a half-loaded frame. Only the newest request is applied: two loads can overlap when a
 * new connection sends both a Resync and a return to online, and an older answer must not win.
 */
function createReloader(ctx: Ctx, data: Data, active: readonly Syncer[], progress: Progress) {
  let newest = 0;
  return async (): Promise<void> => {
    const mine = ++newest;
    try {
      for (const syncer of active) {
        const snapshot = await syncer.load(data.api, ctx);
        if (mine !== newest) return;
        batch(() => {
          syncer.apply(ctx, snapshot);
        });
      }
      if (mine !== newest) return;
      batch(() => {
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
 * The topics a set of sections needs for the projects that exist right now: the ones that do not
 * depend on the store, plus each project's own. A card's events go to `project:<id>`, so the set
 * changes as projects come and go.
 */
function topicsFor(ctx: Ctx, active: readonly Syncer[]): string[] {
  const topics = new Set<string>(active.flatMap((syncer) => syncer.topics));
  for (const project of ctx.S.projects) {
    for (const syncer of active) {
      for (const topic of syncer.projectTopics?.(project.id) ?? []) topics.add(topic);
    }
  }
  return [...topics];
}

/**
 * Keeps the stream's subscriptions in step with the projects that exist: it subscribes to the
 * topics of a project that arrived, and unsubscribes from the topics of one that left. It reads
 * the projects through a signal, so it runs again whenever the project list changes, and it never
 * sends a subscription the stream already has.
 */
function followProjects(ctx: Ctx, data: Data, active: readonly Syncer[]): void {
  // The topics of the projects that are already there are subscribed now, before the stream starts,
  // so the first hello carries them. The effect below only handles projects that come and go.
  let subscribed = new Set<string>(topicsFor(ctx, active));
  data.stream.subscribe([...subscribed]);
  createEffect(() => {
    // Reading the ids (not the projects) is what this effect depends on: a project that was renamed
    // must not re-subscribe to everything.
    const wanted = new Set(topicsFor(ctx, active));
    const added = [...wanted].filter((topic) => !subscribed.has(topic));
    const gone = [...subscribed].filter((topic) => !wanted.has(topic));
    if (added.length > 0) data.stream.subscribe(added);
    if (gone.length > 0) data.stream.unsubscribe(gone);
    subscribed = wanted;
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
  // Reads the open project chat again, which is set once the effects below exist (see
  // `followOpenChat`): what was said while the connection was away is not replayed. `resendTerminal`
  // does the same for the open card's terminal (`card-view.ts`'s `followOpenCardTerminal`), since a
  // card's terminal output is never replayed either (docs/architecture.md 11.2).
  let rereadChat: () => void = () => undefined;
  let resendTerminal: () => void = () => undefined;
  const stops = [
    data.onEvents((events) =>
      batch(() => {
        for (const event of events) {
          // A card's own events come to the panel, not to a section-wide syncer; the two are
          // separate because a card is opened and closed while a section stays switched on.
          applyCardSessionEvent(ctx, event);
          applyChatSessionEvent(ctx, event);
          applyTerminalOutputEvent(ctx, event);
          for (const syncer of active) syncer.onEvent?.(ctx, event);
        }
      }),
    ),
    data.onResync(() => {
      void reload();
      rereadChat();
      resendTerminal();
    }),
    data.connection.onReconnected(() => {
      void reload();
      rereadChat();
      resendTerminal();
    }),
    // A card's terminal channel answered or refused (`card-view.ts`'s `applyTerminalFrame`), for
    // whichever card asked for it — only the open one ever does, since `followOpenCardTerminal`'s
    // snapshot request is the only thing that ever sends one.
    data.onTerminalFrame((frame, cardId) => applyTerminalFrame(ctx, frame, cardId)),
  ];
  const disposeEffects = createRoot((dispose) => {
    followConnection(ctx, data, progress, reload);
    followProjects(ctx, data, active);
    // The open card's own topic, for the card panel: its chat and activity are read when it opens.
    followOpenCard(ctx, data.api, data.stream);
    // The open project chat's own topic, for the Chats view: its history is read when it opens.
    rereadChat = followOpenChat(ctx, data.api, data.stream);
    // The open card's real terminal (section S9), registered after `followOpenCard` so its own
    // topic is already followed by the time this asks for a snapshot on the same socket.
    resendTerminal = followOpenCardTerminal(ctx, data.api, data.stream);
    // What a section does between snapshots, such as saving a preference the person changed.
    const stopped = active.flatMap((syncer) => syncer.start?.(ctx, data.api) ?? []);
    return () => {
      for (const stop of stopped) stop();
      dispose();
    };
  });
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
