import { createMutable } from "solid-js/store";
import type { Data } from "~/data";
import { createOptimistic, type Optimistic } from "~/data/optimistic";
import { platform } from "~/platform";
import { isDaemon, type SectionId, type SectionStatus, sectionStatus } from "~/data/sections";
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
import type { Integration } from "./settings-types";
import { ONBOARDED_KEY } from "./storage";

/** Everything the store needs from its host. The browser values come from `index.ts`. */
export interface Env {
  /** `location.hash`; its flags turn parts of the simulation off. */
  hash: string;
  storage: KeyValueStore | null;
  viewport: { w: number; h: number };
  /** Writes `S.theme` to the document; a no-op outside the browser. */
  applyTheme: (S: State) => void;
  /** Opens the browser's file chooser for an image and resolves with the file, or null when closed. Absent outside the browser. */
  pickImage?: () => Promise<File | null>;
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

/**
 * The mock's GitHub and Obsidian rows once each connection is the daemon's own (S29a, S29b): the
 * prototype's sentence must not show for what the daemon has not actually answered yet. GitHub is a
 * person's own connection with nothing stored until they connect it, so it reads as not connected.
 * Obsidian is Marshal's own - the daemon always has a vault - so it reads as connected with no
 * detail rather than "none", and without the mock's made-up path, until the daemon's first answer
 * names the real one. The app's own words (the name and the icon) stay for both, and each row is
 * only touched when its own section has switched, so the other stays exactly as the seed made it
 * while its section is still the mock's.
 */
function withoutConnectionSeed(
  rows: Integration[],
  table: Readonly<Record<SectionId, SectionStatus>>,
): Integration[] {
  return rows.map((row) => {
    if (row.id === "github" && isDaemon("S29a", table)) return { ...row, st: "none", detail: "" };
    if (row.id === "obsidian" && isDaemon("S29b", table)) {
      return { ...row, st: "connected", detail: "" };
    }
    return row;
  });
}

/** The sections table a store uses: the test's own, or the real one. */
export const sectionsOf = (env: Env): Readonly<Record<SectionId, SectionStatus>> =>
  env.sections ?? sectionStatus;

export function createContext(env: Env): Ctx {
  const loadedAt = Date.now();
  const today = startOfToday();
  const ids = createIds();
  const { cards, chats, notices, feed, ...seed } = buildSeed(ids, loadedAt);
  // A card's chat and its activity are the daemon's once their sections are switched, and this seed
  // is the mock's own: it would otherwise sit behind a real card's history and show through it. The
  // reservoir keeps the cards out the same way (`sync/reservoir.ts`).
  const table = sectionsOf(env);
  const seedless = {
    ...seed,
    ...(isDaemon("S8a", table) ? { chat: {} } : {}),
    ...(isDaemon("S10", table) ? { act: {} } : {}),
    // The provider keys are the daemon's once S28 is switched: only the masked value ever leaves the
    // daemon, so the mock's rows would otherwise show a key that is not stored anywhere.
    ...(isDaemon("S28", table) ? { providers: [] } : {}),
    // GitHub's and Obsidian's rows are the daemon's once S29a and S29b switch, each on its own
    // (withoutConnectionSeed): the prototype's own sentences would otherwise show for what the
    // daemon has not actually answered yet. The connections of later phases (S29c to S29g) are left
    // as the mock's until their own sections switch.
    ...(isDaemon("S29a", table) || isDaemon("S29b", table)
      ? { integrations: withoutConnectionSeed(seed.integrations, table) }
      : {}),
    // The cost and awake limits are the daemon's once S26b is switched, and it ships with none: the
    // mock's four fabricated scopes are dropped, and only `global` stays so `selectors.costs` and the
    // awake section always find a scope to read.
    ...(isDaemon("S26b", table) ? { limits: { global: {} } } : {}),
    // The roles are the daemon's once S27 is switched, and it ships only its own starters: the
    // mock's eight are dropped so they cannot show through the daemon's own list. The screens read
    // an empty list until the first snapshot lands, and `ready` gates drawing until it has.
    ...(isDaemon("S27", table) ? { roles: [] } : {}),
    // The person is the daemon's once S2a is switched: the prototype's Ada, Blair, and the rest are
    // its own, and the profile's name, email, and time zone are filled by `sync/profile.ts`. The
    // tailnet, the node, and the devices stay the mock's until Phase 9.
    ...(isDaemon("S2a", table)
      ? { people: [], profile: { ...seed.profile, name: "", email: "", tz: "", avatar: null } }
      : {}),
  };
  // A project appears only when the daemon has it, and its mock records come out of the reservoir with it.
  const hidden: Reservoir = { cards, chats, notices, feed };
  const S = createMutable(
    initialState(
      { ...seedless, cards: [], chats: {}, notices: [], feed: [] },
      {
        vw: env.viewport.w,
        vh: env.viewport.h,
        // Once the first-launch screens are the daemon's (S31a), whether they show is the daemon's
        // progress, which arrives before the app draws: the skeleton is up until then. So the
        // device's own note is not read, and a person who set up on another device never sees
        // onboarding again.
        onboarded: isDaemon("S31a", table) || !!readKey(env.storage, ONBOARDED_KEY),
        today,
        savedViewsOnDaemon: isDaemon("S6a", table),
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
    optimistic: createOptimistic((message) => {
      toast(ctx, message);
      platform().haptic("error");
    }),
    sync: null,
    flags: { suppressClick: false, ciFailRunning: false, tickN: 0 },
  };
  return ctx;
}
