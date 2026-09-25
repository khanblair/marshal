import { colOf, isAwake, PHONE_MAX_WIDTH_PX } from "./constants";
import type { Ctx } from "./context";
import { relTime } from "./format";
import type { State } from "./state-types";
import type { ApprovalMsg, Card, Chat, Column, Person, PlanMsg, Project } from "./types";

/* Read-only queries on the store. Lists are fresh arrays, because views sort them in place. */

export const isMobile = (S: State): boolean => S.vw < PHONE_MAX_WIDTH_PX;

export const now = (ctx: Ctx): number => ctx.clock.now();

/** Relative time that updates on every simulation tick. */
export const rel = (ctx: Ctx, ts: number): string => relTime(ts, ctx.clock.now());

/** Finds a card by id. Accepts strings too, as the views pass `"#118".slice(1)`. */
export const card = (ctx: Ctx, id: number | string | null | undefined): Card | undefined => {
  const n = Number(id);
  return ctx.S.cards.find((c) => c.id === n);
};

export const proj = (ctx: Ctx, id: string | null | undefined): Project | undefined =>
  ctx.S.projects.find((p) => p.id === id);

export const person = (ctx: Ctx, id: string): Person | undefined =>
  ctx.S.people.find((p) => p.id === id);

export const cardsOf = (ctx: Ctx, pid: string | null): Card[] =>
  ctx.S.cards.filter((c) => c.p === pid);

/** Cards of one column in board order: oldest first in Needs you, newest first in Done, else pinned then newest id. */
export function colCards(list: readonly Card[], col: Column): Card[] {
  return list
    .filter((c) => colOf(c.state) === col)
    .sort((a, b) => {
      if (col === "needs") return a.upd - b.upd;
      if (col === "done") return b.upd - a.upd;
      return Number(b.pinned) - Number(a.pinned) || b.id - a.id;
    });
}

const inProject = (c: Card, pid?: string | null): boolean => !pid || c.p === pid;

export const needs = (ctx: Ctx, pid?: string | null): Card[] =>
  ctx.S.cards.filter((c) => c.state === "needs" && inProject(c, pid)).sort((a, b) => a.upd - b.upd);

export const awake = (ctx: Ctx, pid?: string | null): Card[] =>
  ctx.S.cards.filter((c) => isAwake(c) && inProject(c, pid));

export const working = (ctx: Ctx, pid?: string | null): Card[] =>
  ctx.S.cards.filter((c) => c.state === "working" && inProject(c, pid));

export interface Costs {
  today: number;
  month: number;
  day: number;
  monthL: number;
  awakeL: number;
}

/** Share of open-card spend counted as today's, plus the fixed Orchestrator spend. */
const TODAY_SHARE = 0.92;
const ORCHESTRATOR_TODAY_USD = { project: 0.42, all: 1.26 };

export function costs(ctx: Ctx, pid?: string | null): Costs {
  const { S } = ctx;
  const open = S.cards.filter((c) => inProject(c, pid) && c.state !== "done");
  const spent = open.reduce((a, c) => a + c.cost, 0);
  const today =
    spent * TODAY_SHARE + (pid ? ORCHESTRATOR_TODAY_USD.project : ORCHESTRATOR_TODAY_USD.all);
  const base = pid
    ? (proj(ctx, pid)?.monthBase ?? 0)
    : S.projects.reduce((a, p) => a + p.monthBase, 0);
  const limits = (pid && S.limits[pid]) || S.limits.global;
  return {
    today,
    month: base + today,
    day: limits.day,
    monthL: limits.month,
    awakeL: limits.awake,
  };
}

const NEAR_LIMIT_RATIO = 0.8;

export function costTone(value: number, limit: number): "over" | "near" | "normal" {
  const r = value / limit;
  if (r >= 1) return "over";
  return r >= NEAR_LIMIT_RATIO ? "near" : "normal";
}

/** The approval or plan a card is waiting on, if any. */
export const pendingApproval = (ctx: Ctx, id: number): ApprovalMsg | PlanMsg | undefined =>
  (ctx.S.chat[id] ?? []).find(
    (x): x is ApprovalMsg | PlanMsg => (x.k === "approval" || x.k === "plan") && x.st === "waiting",
  );

export const chatsOf = (ctx: Ctx, pid: string): Chat[] =>
  [...(ctx.S.chats[pid] ?? [])].sort((a, b) => b.last - a.last);

export const chatById = (ctx: Ctx, pid: string, id: string | null | undefined): Chat | undefined =>
  (ctx.S.chats[pid] ?? []).find((c) => c.id === id);

const DUPE_MIN_WORD_LENGTH = 4;
const DUPE_MIN_SHARED_WORDS = 2;
const MAX_DUPES = 2;

/** Open cards in the current project whose titles share two or more longer words with `title`. */
export function dupes(ctx: Ctx, title: string): Card[] {
  const words = title
    .toLowerCase()
    .split(/\W+/)
    .filter((w) => w.length >= DUPE_MIN_WORD_LENGTH);
  if (words.length < DUPE_MIN_SHARED_WORDS) return [];
  const { S } = ctx;
  return S.cards
    .filter((c) => c.p === S.route.pid && c.state !== "done")
    .filter((c) => {
      const t = c.title.toLowerCase();
      return words.filter((w) => t.includes(w)).length >= DUPE_MIN_SHARED_WORDS;
    })
    .slice(0, MAX_DUPES);
}
