/**
 * Text and rules of the timeline that do not depend on the store: bar tooltips, what a
 * drop would break, and the day groups of the phone list. From design/TimelineView.dc.html.
 */
import { type CardKey, cardLabel, cardNumber } from "~/mock/card-key";
import type { Planned } from "./timeline-geometry";

interface Titled extends Planned {
  title: string;
}

/** `#39, #41` for the card keys, which belong to the timeline's own project. */
const refs = (keys: readonly CardKey[]): string =>
  keys.map((key) => cardLabel({ n: cardNumber(key) })).join(", ");

/** Bar tooltip: the card, its state, what it waits for, and what waits for it. */
export function barTip(c: Titled, stateLabel: string, blocks: readonly Planned[]): string {
  const dependsOn = c.deps.length ? `. Depends on ${refs(c.deps)}` : "";
  const blocking = blocks.length ? `. Blocks ${refs(blocks.map((b) => b.id))}` : "";
  return `${cardLabel(c)} ${c.title}. ${stateLabel}${dependsOn}${blocking}`;
}

/** Cards on the timeline, sorted by planned start and then id. */
export function timelineCards<T extends Planned>(cards: readonly T[]): T[] {
  return cards.filter((c) => c.s != null).sort((a, b) => (a.s ?? 0) - (b.s ?? 0) || a.n - b.n);
}

export interface MovePlan {
  s: number;
  e: number;
  /** One sentence per dependency the move would newly break. */
  broken: string[];
}

/**
 * The new dates of card `c` moved by `delta` days, and the dependencies that would
 * newly break: `c` starting before a card it waits for ends, or a card waiting for `c`
 * starting before `c` ends. Already broken dependencies are not reported again.
 */
export function planMove<T extends Planned>(
  c: T,
  delta: number,
  all: readonly T[],
  find: (id: CardKey) => T | undefined,
): MovePlan {
  const s0 = c.s ?? 0;
  const e0 = c.e ?? 0;
  const s = s0 + delta;
  const e = e0 + delta;
  const broken: string[] = [];
  for (const id of c.deps) {
    const a = find(id);
    if (a && a.e != null && s <= a.e && !(s0 <= a.e)) {
      broken.push(`${cardLabel(c)} would start before ${cardLabel(a)} finishes`);
    }
  }
  for (const b of all.filter((x) => x.deps.includes(c.id))) {
    if (b.s != null && b.s <= e && !(b.s <= e0)) {
      broken.push(`${cardLabel(b)} would start before ${cardLabel(c)} finishes`);
    }
  }
  return { s, e, broken };
}

export const breakMessage = (broken: readonly string[]): string =>
  `${broken.join(". ")}. The dependent card still waits for the merge before it starts.`;

export interface DayGroup<T> {
  offset: number;
  cards: T[];
}

/** Cards grouped by planned start day, earliest day first. Keeps the cards' order. */
export function groupByStart<T extends Planned>(cards: readonly T[]): DayGroup<T>[] {
  const byDay = new Map<number, T[]>();
  for (const c of cards) {
    const day = c.s ?? 0;
    byDay.set(day, [...(byDay.get(day) ?? []), c]);
  }
  return [...byDay.entries()]
    .sort(([a], [b]) => a - b)
    .map(([offset, list]) => ({ offset, cards: list }));
}

/** "Thu, Sep 24" for a day offset from today. */
export const shortDay = (today: number, dayMs: number, offset: number): string =>
  new Date(today + offset * dayMs).toLocaleDateString(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
  });

/** Heading of a phone day group. */
export const groupLabel = (today: number, dayMs: number, offset: number): string =>
  `${offset === 0 ? "Today, " : ""}${shortDay(today, dayMs, offset)}`;

/** "One day", or "Until Mon, Sep 28". */
export const spanLabel = (c: Planned, today: number, dayMs: number): string =>
  c.e === c.s ? "One day" : `Until ${shortDay(today, dayMs, c.e ?? 0)}`;

/** A card `c` waits for is late when it ends on or after the day `c` starts. */
export const isLateFor = (a: Planned, c: Planned): boolean => (a.e ?? 0) >= (c.s ?? 0);
