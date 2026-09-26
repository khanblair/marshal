/**
 * The search route of the fake daemon (docs/backend-checklist.md B2.11), so the unit and component
 * tests of the palette can drive the real API client and the real search session without a
 * network. It follows the real service's rules in miniature: the query is trimmed and its white
 * space collapsed (and answered back as `query`), every word must match, in any order, ignoring
 * case, and `#41`, `api#41`, and `41` name a card by its number. It ranks by where the words sit
 * (a title above a description) and cuts each kind to `SearchHitsPerKind`, with `totals` counting
 * what matched before the cut. Only live chats are searched.
 */
import {
  type Card,
  type Chat,
  MaxSearchQueryChars,
  type Project,
  SearchHitsPerKind,
  type SearchSnapshot,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";

const BAD_REQUEST = 400;

/** What the route reads: the daemon's projects, cards, and chats, as the other fake routes hold them. */
export interface SearchStore {
  projects: readonly Project[];
  cards: readonly Card[];
  chats: readonly Chat[];
  now: () => string;
}

/** How well a thing matched: 0 for no match, and more for a better place (a title beats a body). */
type Score = number;

const TITLE = 4;
const KEY = 2;
const BODY = 1;

/** A word that is the whole key, or the digits of the number, outranks every text match. */
const WHOLE_KEY = 100;
const NUMBER = 90;

/** A word's score against a card's number, or 0 when the word does not name a number. */
function numberScore(card: Card, word: string): Score {
  const digits = word.replace(/^#/, "");
  if (word === card.key.toLowerCase()) return WHOLE_KEY;
  if (/^\d+$/.test(digits)) return String(card.number).startsWith(digits) ? NUMBER : 0;
  return 0;
}

/** The best score of a word in the given fields, each with its weight. */
function best(word: string, fields: readonly [text: string, weight: number][]): Score {
  return Math.max(
    0,
    ...fields.map(([text, weight]) => (text.toLowerCase().includes(word) ? weight : 0)),
  );
}

/** The sum of every word's best score, or 0 when a word matches nothing. */
function sum(words: readonly string[], score: (word: string) => Score): Score {
  let total = 0;
  for (const word of words) {
    const one = score(word);
    if (one === 0) return 0;
    total += one;
  }
  return total;
}

const projectScore = (p: Project, words: readonly string[]): Score =>
  sum(words, (w) =>
    best(w, [
      [p.name, TITLE],
      [p.id, TITLE],
      [p.path, BODY],
    ]),
  );

const cardScore = (c: Card, words: readonly string[]): Score =>
  sum(words, (w) =>
    Math.max(
      numberScore(c, w),
      best(w, [
        [c.title, TITLE],
        [c.key, KEY],
        [c.body, BODY],
      ]),
    ),
  );

const chatScore = (c: Chat, words: readonly string[]): Score =>
  sum(words, (w) => best(w, [[c.title, TITLE]]));

/** The things that matched, best first and in the store's order among equals, with how many there were. */
function ranked<T>(things: readonly T[], score: (thing: T) => Score): { top: T[]; total: number } {
  const scored = things
    .map((thing, order) => ({ thing, order, score: score(thing) }))
    .filter((one) => one.score > 0)
    .sort((a, b) => b.score - a.score || a.order - b.order);
  return { top: scored.slice(0, SearchHitsPerKind).map((one) => one.thing), total: scored.length };
}

const cleaned = (text: string): string => text.trim().replace(/\s+/g, " ");

/** The answer for a cleaned query, from what the store holds. */
function answerFor(store: SearchStore, query: string): SearchSnapshot {
  const words = query ? query.toLowerCase().split(" ") : [];
  const nameOf = (id: string): string => store.projects.find((p) => p.id === id)?.name ?? id;
  const projects = ranked(store.projects, (p) => (words.length ? projectScore(p, words) : 0));
  const cards = ranked(store.cards, (c) => (words.length ? cardScore(c, words) : 0));
  const chats = ranked(
    store.chats.filter((c) => !c.archivedAt),
    (c) => (words.length ? chatScore(c, words) : 0),
  );
  return {
    query,
    projects: projects.top.map((p) => ({
      projectId: p.id,
      name: p.name,
      path: p.path,
      language: p.language,
    })),
    cards: cards.top.map((c) => ({
      cardId: c.id,
      key: c.key,
      number: c.number,
      title: c.title,
      state: c.state,
      projectId: c.projectId,
      projectName: nameOf(c.projectId),
    })),
    chats: chats.top.map((c) => ({
      chatId: c.id,
      title: c.title,
      projectId: c.projectId,
      projectName: nameOf(c.projectId),
      lastActiveAt: c.lastActiveAt,
    })),
    totals: { projects: projects.total, cards: cards.total, chats: chats.total },
    serverTime: store.now(),
  };
}

/** Answers `GET /v1/search?q=`, or returns undefined for any other request. */
export function answerSearchRoute(store: SearchStore, request: FakeRequest): Response | undefined {
  const [path, query = ""] = request.url.replace(/^https?:\/\/[^/]+/, "").split("?");
  if (path !== "/v1/search" || request.method !== "GET") return undefined;
  const text = cleaned(new URLSearchParams(query).get("q") ?? "");
  if ([...text].length > MaxSearchQueryChars) {
    return errorAnswer(BAD_REQUEST, "invalid_argument", "Search for 200 characters or fewer.");
  }
  return jsonAnswer(answerFor(store, text));
}
