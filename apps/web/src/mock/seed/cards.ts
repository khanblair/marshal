import { MINUTE_MS } from "../constants";
import type { Card } from "../types";
import { API_CARDS } from "./cards-api";
import { MOBILE_CARDS } from "./cards-mobile";
import { WEB_CARDS } from "./cards-web";

/** Card fields for `makeCard`. `upd` here is minutes before load, as in the prototype's `C()`. */
export type CardSeed = Omit<Partial<Card>, "upd"> &
  Pick<Card, "id" | "p" | "title" | "state"> & { upd?: number };

const DEFAULT_UPD_MIN = 30;

/**
 * Same defaults as the prototype's `C()`. `upd` counts back from load time, not the current time.
 * The seed is copied so no two store instances share an array.
 */
export function makeCard(loadedAt: number, seed: CardSeed): Card {
  return {
    role: "Worker",
    agent: "Claude Code",
    model: "claude-sonnet-4-5",
    think: "Medium",
    perm: "Auto-accept edits",
    branch: null,
    ci: null,
    cost: 0,
    doing: "",
    reason: "",
    asleep: false,
    pinned: false,
    bypass: false,
    paused: false,
    pkg: null,
    labels: [],
    deps: [],
    members: [],
    checklists: [],
    comments: [],
    s: null,
    e: null,
    due: null,
    ctx: 0.22,
    pr: null,
    mergePct: 0,
    waking: false,
    ...structuredClone(seed),
    upd: loadedAt - (seed.upd ?? DEFAULT_UPD_MIN) * MINUTE_MS,
  };
}

/** Seed cards in the prototype's order; the order decides the seeded message ids. */
export const seedCards = (loadedAt: number): Card[] =>
  [...API_CARDS, ...WEB_CARDS, ...MOBILE_CARDS].map((seed) => makeCard(loadedAt, seed));
