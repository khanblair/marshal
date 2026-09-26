/**
 * The Home routes of the fake daemon: the numbers the dashboard draws (section S19a), built from
 * the cards' own states and their sessions, and the activity stream beside them (section S20),
 * newest first and paged by cursor.
 */
import type {
  FeedEntry,
  HomeSnapshot,
  HomeStatDay,
  HomeStats,
  Page,
  SessionState,
} from "@marshal/protocol";
import { jsonAnswer } from "~/data/testing/fake-fetch";
import type { CardStore } from "./fake-card-shared";

/** How many rows a page of the activity stream holds when the caller does not say. */
const DEFAULT_ACTIVITY_LIMIT = 50;

/**
 * One page of the Home activity stream (section S20), newest first, narrowed by kind and project
 * and paged by cursor: a row's place in `store.activity`, counting from the end, is its seq (the
 * oldest row is seq 1), the same "pass the seq the previous page ended on" cursor the real daemon
 * uses. A test seeds `store.activity` newest first, the order `FakeDaemonOptions.activity` takes.
 */
export function homeActivity(store: CardStore, query: URLSearchParams): Response {
  const seqOf = (index: number): number => store.activity.length - index;
  const kind = query.get("kind");
  const project = query.get("project");
  const cursor = query.get("cursor");
  const before = cursor ? Number(cursor) : undefined;
  const limit = Number(query.get("limit")) || DEFAULT_ACTIVITY_LIMIT;
  const matches = store.activity
    .map((entry, index) => ({ entry, seq: seqOf(index) }))
    .filter(
      (row) =>
        (before === undefined || row.seq < before) &&
        (!kind || row.entry.kind === kind) &&
        (!project || row.entry.projectId === project),
    );
  const page = matches.slice(0, limit);
  const nextCursor = matches.length > limit ? String(page.at(-1)?.seq ?? "") : "";
  const answer: Page<FeedEntry> = {
    items: page.map((row) => row.entry),
    nextCursor,
    serverTime: store.now(),
  };
  return jsonAnswer(answer);
}

const HOURS_PER_DAY = 24;
const MINUTES_PER_HOUR = 60;
const SECONDS_PER_MINUTE = 60;
const MS_PER_SECOND = 1000;
const MS_PER_DAY = HOURS_PER_DAY * MINUTES_PER_HOUR * SECONDS_PER_MINUTE * MS_PER_SECOND;
/** The three ranges the charts cover (section S19a). */
const DEFAULT_STATS_RANGE = 7;

/** Midnight, in UTC, of the moment the store's own clock reads. */
function todayMidnight(store: CardStore): number {
  const now = new Date(store.now());
  return Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate());
}

/**
 * The stored Home numbers for a range, all zeroed except today, which counts the cards this store
 * already has in Done: enough for a test to prove the chart reads real numbers, not that this fake
 * tracks history the way `daily_stats` does.
 */
function homeStats(store: CardStore, range: number): HomeStats {
  const today = todayMidnight(store);
  const finishedToday = store.cards.filter((card) => card.state === "done").length;
  const days: HomeStatDay[] = Array.from({ length: range }, (_, i) => {
    const day = new Date(today - (range - 1 - i) * MS_PER_DAY).toISOString();
    const isToday = i === range - 1;
    return {
      day,
      cardsFinished: isToday ? finishedToday : 0,
      merges: isToday ? finishedToday : 0,
      ciFailures: 0,
      costMicros: 0,
    };
  });
  return {
    range,
    from: days[0]?.day ?? new Date(today).toISOString(),
    to: new Date(today).toISOString(),
    days,
    projects: [],
  };
}

/**
 * The states of a stored session that Home's awake list reads (`ListAwakeCards` in the daemon's
 * dashboard.sql): the list is the cards whose session is awake, working, or waking.
 */
const HOME_AWAKE: readonly SessionState[] = ["awake", "working", "waking"];

/** The numbers Home draws, built from the cards' own states and their sessions. */
export function homeDashboard(
  store: CardStore,
  projectNames: (id: string) => string,
  query: URLSearchParams,
): Response {
  const range = Number(query.get("range")) || DEFAULT_STATS_RANGE;
  const needs = store.cards.filter((card) => card.state === "needs");
  const awake = store.cards.filter((card) => card.session && HOME_AWAKE.includes(card.session));
  const snapshot: HomeSnapshot = {
    needs: needs.map((card) => ({
      cardId: card.id,
      key: card.key,
      number: card.number,
      projectId: card.projectId,
      projectName: projectNames(card.projectId),
      title: card.title,
      reason: card.needsReason ?? { kind: "question", text: "" },
      waitingSince: card.updatedAt,
      role: card.role,
    })),
    awake: awake.map((card) => ({
      cardId: card.id,
      key: card.key,
      number: card.number,
      projectId: card.projectId,
      projectName: projectNames(card.projectId),
      title: card.title,
      state: card.state,
      session: card.session ?? "awake",
      doingNow: card.doingNow,
      contextUsed: card.contextUsed,
      pinned: card.pinned,
      paused: card.paused,
    })),
    tiles: { needs: needs.length, working: awake.length, mergedToday: 0 },
    stats: homeStats(store, range),
    serverTime: store.now(),
  };
  return jsonAnswer(snapshot);
}
