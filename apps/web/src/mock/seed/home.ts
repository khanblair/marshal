// biome-ignore-all lint/style/noMagicNumbers: seed data table, values are the prototype's fake data
import { MINUTE_MS } from "../constants";
import { type IdCounters, takeMid } from "../ids";
import type { FeedItem, Notice } from "../types";

/** Idle cards in the seeded sleep notice go to sleep this long after load. */
const SLEEP_NOTICE_MS = 125_000;

export const seedNotices = (loadedAt: number): Notice[] => [
  {
    id: "n1",
    kind: "sleep",
    cards: ["api#39", "api#36", "web#116"],
    deadline: loadedAt + SLEEP_NOTICE_MS,
    ts: loadedAt,
  },
  {
    id: "n2",
    kind: "ci-main",
    pid: "mobile",
    cardId: "mobile#213",
    text: "Main is failing in mobile-app",
    sub: "The android workflow failed in LoginFlowTest",
    ts: loadedAt - 22 * MINUTE_MS,
  },
  {
    id: "n3",
    kind: "cost",
    pid: "mobile",
    text: "mobile-app is near today's cost limit",
    sub: "Running cards pause when it reaches $8.00",
    ts: loadedAt - 9 * MINUTE_MS,
  },
];

type FeedSeed = Omit<FeedItem, "id" | "ts"> & { ago: number };

const HOURS = 60;
const DAYS = 24 * HOURS;

/* Oldest entry first; each new entry goes on top, so the feed ends up newest first. */
const FEED: FeedSeed[] = [
  { kind: "brief", text: "Morning brief is ready", pid: null, job: "s1", ago: 6 * HOURS },
  {
    kind: "merge",
    text: "#205 Refresh tokens in secure storage merged into main",
    pid: "mobile",
    cardId: "mobile#205",
    ago: 3 * DAYS,
  },
  {
    kind: "merge",
    text: "#110 Fix flaky login e2e test merged into main",
    pid: "web",
    cardId: "web#110",
    ago: 26 * HOURS,
  },
  {
    kind: "schedule",
    text: "Check open issues ran and sent 2 issues to the Orchestrator",
    pid: "web",
    job: "s3",
    ago: 5 * HOURS,
  },
  {
    kind: "approval",
    text: "You approved pnpm add @tanstack/react-table on #119",
    pid: "web",
    cardId: "web#119",
    ago: 70,
  },
  { kind: "plan", text: "Plan ready for review on #43", pid: "api", cardId: "api#43", ago: 34 },
  {
    kind: "ci",
    text: "CI failed on main in mobile-app: android workflow",
    pid: "mobile",
    cardId: "mobile#213",
    ago: 22,
  },
  { kind: "ci", text: "CI passed on marshal/39-slog", pid: "api", cardId: "api#39", ago: 16 },
];

export function seedFeed(ids: IdCounters, loadedAt: number): FeedItem[] {
  const feed: FeedItem[] = [];
  for (const { ago, ...item } of FEED) {
    feed.unshift({ id: `f${takeMid(ids)}`, ...item, ts: loadedAt - ago * MINUTE_MS });
  }
  return feed;
}
