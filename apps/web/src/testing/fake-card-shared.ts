/**
 * What every one of the fake daemon's card routes shares: the store they change, the wire fixture a
 * card is made from, the opaque ids a fixture card and a made card are given, the publish a change
 * is announced with, the refusal the daemon's own words are put in, and the paging a list route
 * reads. The route groups live beside this file: fake-cards holds the board and the routing,
 * fake-card-actions the card's own actions and reads, fake-card-labels the project's labels, and
 * fake-home the numbers Home draws.
 *
 * Only the fields the daemon fills are made here. The shapes come from the golden files, which the
 * Go tests wrote from the real wire types.
 */
import type {
  ActivityItem,
  CardState,
  ChangedFile,
  ChatMessage,
  DiffHunk,
  FeedEntry,
  Label,
  Card as WireCard,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";

/** The board's columns, in the order the daemon answers them. */
export const BOARD_COLUMNS: readonly CardState[] = [
  "backlog",
  "planning",
  "working",
  "needs",
  "review",
  "ready",
  "done",
];

export const STATUS = { created: 201, badRequest: 400, notFound: 404, conflict: 409, refused: 422 };

/** A card the fake daemon holds: the wire type, and the opaque id its routes use. */
export function wireCard(
  fields: Partial<WireCard> & { projectId: string; number: number },
): WireCard {
  const base = structuredClone(golden<WireCard>("card"));
  const key = `${fields.projectId}#${fields.number}`;
  return {
    ...base,
    id: cardId(fields.number),
    key,
    ...fields,
    projectId: fields.projectId,
    number: fields.number,
  };
}

/** How many digits a card's number takes in the id a fixture card is given. */
const ID_DIGITS = 6;

/**
 * The opaque id a fixture card is given, from its number. A card the fake daemon *makes* gets one
 * from `freshCardId` instead, because two projects can hold the same number and the real daemon's
 * ids are unique across every project.
 */
export function cardId(number: number): string {
  return `01M3CARD000000000000${String(number).padStart(ID_DIGITS, "0")}`;
}

/** Where the ids of cards the fake daemon makes start, so they cannot collide with a fixture's. */
const MADE_CARD_BASE = 900_000;

/** How many ids the fake daemon has handed out: one per card it made, so no two cards share one. */
let madeCards = 0;

/** A fresh opaque id, unique among the cards this fake daemon made. */
export function freshCardId(): string {
  madeCards += 1;
  return cardId(MADE_CARD_BASE + madeCards);
}

/**
 * One project's board as the daemon answers it, for a test that applies a snapshot without a
 * daemon.
 */
export function boardOf(projectId: string, cards: readonly WireCard[]): DaemonBoard {
  return { projectId, cards: [...cards].sort((a, b) => a.number - b.number) };
}

/** The boards of every project the daemon holds, in the daemon's own order. */
export interface DaemonBoard {
  projectId: string;
  cards: WireCard[];
}

export const notFound = () =>
  errorAnswer(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that card. It may have been removed.",
  );

/**
 * A refusal, as the daemon answers one: the code says the change was refused, and the stable
 * reason a client branches on is in the details, which is what `protocol.Refused(...).With` sends.
 */
export const refused = (reason: string, message: string): Response =>
  jsonAnswer({ error: { code: "refused", message, details: { reason } } }, STATUS.refused);

/** What the card routes change and tell the stream about. */
export interface CardStore {
  /** Every card the daemon holds, across projects. */
  cards: WireCard[];
  /** Every label the daemon holds, across projects. */
  labels: Label[];
  /** The stored history of every card, as the daemon's own index keeps it: newest first per card. */
  history: HistoryRow[];
  /** The Home activity stream (section S20), newest first, as `GET /v1/home/activity` pages it. */
  activity: FeedEntry[];
  /** A card's diff (section S11), by card id: the changed files, and each one's hunks by path. */
  diffs: Record<string, FakeCardDiff>;
  /** Sends an event on a topic, as the daemon does after a change. */
  publish: (topic: string, type: string, data: unknown) => void;
  now: () => string;
}

/** One card's diff, the way a test sets it up: the changed files, and the hunks of the ones it
 * wants opened. A file with no entry in `hunks` answers an empty list, the way an untouched or a
 * binary file's would. */
export interface FakeCardDiff {
  base?: string;
  branch?: string;
  files: ChangedFile[];
  hunks?: Record<string, DiffHunk[]>;
}

export const topic = (projectId: string): string => `project:${projectId}`;

/** One stored row of a card's history: the chat message it draws, and the activity entry beside it. */
export interface HistoryRow {
  cardId: string;
  message: ChatMessage;
  /** The activity entry the same moment draws, when it draws one at all. */
  activity: ActivityItem | null;
}

/** A stored history row a test sets up: the card it belongs to, and what the daemon recorded. */
export function historyRow(
  cardId: string,
  message: Partial<ChatMessage> & Pick<ChatMessage, "id" | "kind">,
  activity: ActivityItem | null = null,
): HistoryRow {
  return {
    cardId,
    message: {
      seq: 1,
      at: "2026-09-26T12:00:00.000Z",
      text: "",
      tool: null,
      diff: null,
      plan: null,
      approval: null,
      card: null,
      ...message,
    },
    activity,
  };
}

/**
 * The card a route names. The daemon takes its own opaque id and nothing else: a key, however
 * natural it looks, is a not_found there, and this fake must answer the same way or a client bug
 * that sends one would pass every test here.
 */
export const cardOf = (store: CardStore, id: string): WireCard | undefined =>
  store.cards.find((card) => card.id === id);

/** The card as it now is, which is what every card event carries. */
export function publishCard(
  store: CardStore,
  card: WireCard,
  type: string,
  extra: object = {},
): void {
  store.publish(topic(card.projectId), type, { card, ...extra });
}

/** One request, and the store it is answered from. */
export interface Route {
  store: CardStore;
  request: FakeRequest;
  method: string;
}

/** The query a list route was asked with, which is where its size, its cursor, and its kind live. */
export function queryOf(url: string): Map<string, string> {
  const [, search = ""] = url.split("?");
  return new Map(
    search
      .split("&")
      .filter(Boolean)
      .map((pair) => {
        const [name = "", value = ""] = pair.split("=");
        return [name, decodeURIComponent(value)] as const;
      }),
  );
}

/** One page of a list, newest first, read from `after` (the cursor's own place) onward. */
export function pageOf<T extends { seq?: number }>(
  rows: readonly T[],
  limit: number,
  cursor: string,
  at: string,
) {
  const after = cursor ? Number(cursor) || 0 : 0;
  const rest = rows.filter((row) => (row.seq ?? 0) > after).slice(0, Math.min(limit, MAX_PAGE));
  const last = rest.at(-1)?.seq ?? 0;
  const more = rows.some((row) => (row.seq ?? 0) > last);
  return { items: rest, nextCursor: more ? String(last) : "", serverTime: at };
}

/** How many rows one page holds at most, and what a page holds when the client does not say. */
const MAX_PAGE = 200;
export const DEFAULT_LIMIT = 50;
