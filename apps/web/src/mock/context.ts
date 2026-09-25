import { createMutable } from "solid-js/store";
import { type Clock, createClock } from "./clock";
import { createIds, type IdCounters } from "./ids";
import { buildSeed } from "./seed";
import { createMsgFactory, type MsgFactory } from "./seed/messages";
import { initialState } from "./state";
import type { State } from "./state-types";
import { type KeyValueStore, ONBOARDED_KEY, readKey } from "./storage";

/** Everything the store needs from its host. The browser values come from `index.ts`. */
export interface Env {
  /** `location.hash`; its flags turn parts of the simulation off. */
  hash: string;
  storage: KeyValueStore | null;
  viewport: { w: number; h: number };
  /** Writes `S.theme` to the document; a no-op outside the browser. */
  applyTheme: (S: State) => void;
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

export function createContext(env: Env): Ctx {
  const loadedAt = Date.now();
  const today = startOfToday();
  const ids = createIds();
  const seed = buildSeed(ids, loadedAt);
  const S = createMutable(
    initialState(seed, {
      vw: env.viewport.w,
      vh: env.viewport.h,
      onboarded: !!readKey(env.storage, ONBOARDED_KEY),
      today,
    }),
  );
  return {
    S,
    loadedAt,
    today,
    ids,
    msg: createMsgFactory(ids),
    clock: createClock(),
    env,
    flags: { suppressClick: false, ciFailRunning: false, tickN: 0 },
  };
}
