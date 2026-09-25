import { createMutable } from "solid-js/store";
import type { Data } from "~/data";
import { createOptimistic, type Optimistic } from "~/data/optimistic";
import { type SectionId, type SectionStatus, sectionStatus } from "~/data/sections";
import { type KeyValueStore, readKey } from "~/data/storage";
import type { Reservoir } from "~/sync/reservoir";
import type { SyncControl } from "~/sync/sync-control";
import { type Clock, createClock } from "./clock";
import { toast } from "./engine";
import { createIds, type IdCounters } from "./ids";
import { buildSeed } from "./seed";
import { createMsgFactory, type MsgFactory } from "./seed/messages";
import { initialState } from "./state";
import type { State } from "./state-types";
import { ONBOARDED_KEY } from "./storage";

/** Everything the store needs from its host. The browser values come from `index.ts`. */
export interface Env {
  /** `location.hash`; its flags turn parts of the simulation off. */
  hash: string;
  storage: KeyValueStore | null;
  viewport: { w: number; h: number };
  /** Writes `S.theme` to the document; a no-op outside the browser. */
  applyTheme: (S: State) => void;
  /** The connection to the daemon. Null (or left out) in a test that needs none: nothing is fetched. */
  data?: Data | null;
  /** Which sections read from the daemon. `sectionStatus` unless a test picks its own table. */
  sections?: Readonly<Record<SectionId, SectionStatus>>;
}

/** One store instance: state, id counters, clock, and simulation flags. */
export interface Ctx {
  S: State;
  /** `Date.now()` when the store was created; seeded times count back from it. */
  loadedAt: number;
  /** Midnight today. */
  today: number;
  ids: IdCounters;
  msg: MsgFactory;
  clock: Clock;
  env: Env;
  /**
   * Mock records of projects the daemon does not have (cards, chats, notices, and feed items). They
   * are kept here, out of `S`, so nothing mock ever shows for a project that does not exist.
   */
  hidden: Reservoir;
  /** Runs a change on the screen first and puts it back when the daemon says no. */
  optimistic: Optimistic;
  /** How the store follows the daemon, set by `startSync`. Null while nothing is synced. */
  sync: SyncControl | null;
  flags: {
    /** Set during a card drag so the click that ends it does not open the card. */
    suppressClick: boolean;
    ciFailRunning: boolean;
    tickN: number;
  };
}

function startOfToday(): number {
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

/** The sections table a store uses: the test's own, or the real one. */
export const sectionsOf = (env: Env): Readonly<Record<SectionId, SectionStatus>> =>
  env.sections ?? sectionStatus;

export function createContext(env: Env): Ctx {
  const loadedAt = Date.now();
  const today = startOfToday();
  const ids = createIds();
  const { cards, chats, notices, feed, ...seed } = buildSeed(ids, loadedAt);
  // A project appears only when the daemon has it, and its mock records come out of the reservoir with it.
  const hidden: Reservoir = { cards, chats, notices, feed };
  const S = createMutable(
    initialState(
      { ...seed, cards: [], chats: {}, notices: [], feed: [] },
      {
        vw: env.viewport.w,
        vh: env.viewport.h,
        onboarded: !!readKey(env.storage, ONBOARDED_KEY),
        today,
      },
    ),
  );
  const ctx: Ctx = {
    S,
    loadedAt,
    today,
    ids,
    msg: createMsgFactory(ids),
    clock: createClock(() => env.data?.clock.offsetMs() ?? 0),
    env,
    hidden,
    optimistic: createOptimistic((message) => toast(ctx, message)),
    sync: null,
    flags: { suppressClick: false, ciFailRunning: false, tickN: 0 },
  };
  return ctx;
}
