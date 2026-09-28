/**
 * The notice routes of the fake daemon (docs/backend-checklist.md B5.6, section S23). Every route
 * answers the way the daemon's own handler does: `GET /v1/notices` is the whole standing list, a
 * notice's own call is one body with one of four action names and answers how many cards it changed,
 * and `DELETE /v1/notices/{id}` clears a notice without touching a card.
 *
 * The list is seeded from the golden `notice-list` the Go tests wrote, so a screen test reads the
 * shape the real daemon sends: one sleep group, `sleep:web-dashboard`, naming two cards by their
 * opaque ids and carrying the moment they sleep.
 *
 * The daemon owns the notices, so a call never draws its change on the client alone: after a call
 * that changes the list the store publishes the whole list again as `notice.dismissed` on the home
 * topic, exactly the way `session.publishNotices` does, and the panel redraws from that one event.
 * A call that changes nothing publishes nothing, which is what dismissing a notice that is already
 * gone does.
 */
import {
  type Notice,
  NoticeActionKeepAll,
  NoticeActionKeepAwake,
  NoticeActionSleepAll,
  NoticeActionSleepNow,
  type NoticeActionResult,
  type NoticeList,
  EventTypeNoticeDismissed,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, emptyAnswer, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";

/** Tells the event stream about a change, as the daemon does after it. The home topic and the four
 * notice calls are the only topic and events this file knows. */
type Publish = (topic: string, type: string, data: unknown) => void;

const STATUS = { badRequest: 400 };
const HOME_TOPIC = "home";
const MAX_ECHOED_ID_BYTES = 256;

const LIST_PATH = "/v1/notices";
const ACTION_PATH = /^\/v1\/notices\/([^/]+)\/actions$/;
const ONE_PATH = /^\/v1\/notices\/([^/]+)$/;

/** The notices and the stream they announce themselves on, as the fake daemon holds them. */
export interface NoticesStore {
  /** Every standing notice. A sleep group is one per project. */
  notices: Notice[];
  /** Announces a changed list to every open stream, as the daemon's own bus does. */
  publish: Publish;
  /** Its clock, as the ISO string `serverTime` and a notice's own time are written from. */
  now: () => string;
}

export interface FakeNoticeOptions {
  /** The notices it starts with. The golden `notice-list` by default. */
  notices?: readonly Notice[];
  /** Where a changed list is announced. Discarded by default, for a test that reads the store. */
  publish?: Publish;
  /** Its clock, as an ISO string. The wall clock by default. */
  now?: () => string;
}

export function createNoticesStore(options: FakeNoticeOptions = {}): NoticesStore {
  return {
    notices: structuredClone([...(options.notices ?? golden<NoticeList>("notice-list").notices)]),
    publish: options.publish ?? (() => undefined),
    now: options.now ?? (() => new Date().toISOString()),
  };
}

/** Answers one notice route, or null when the request is not one. */
export function answerNoticeRoute(store: NoticesStore, request: FakeRequest): Response | null {
  const path = pathOf(request.url);
  if (path === LIST_PATH && request.method === "GET") return listAnswer(store);
  const action = ACTION_PATH.exec(path);
  if (action && request.method === "POST") {
    return noticeAction(store, decodeURIComponent(action[1] ?? ""), request);
  }
  const one = ONE_PATH.exec(path);
  if (one && request.method === "DELETE") {
    return dismiss(store, decodeURIComponent(one[1] ?? ""));
  }
  return null;
}

const pathOf = (url: string): string => url.replace(/^https?:\/\/[^/]+/, "");

function bodyOf(request: FakeRequest): Record<string, unknown> {
  try {
    return request.body ? (JSON.parse(request.body) as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

const asText = (value: unknown): string => (typeof value === "string" ? value : "");

const listAnswer = (store: NoticesStore): Response => jsonAnswer({ notices: store.notices });

/** Announces the list as it now stands, the way `session.publishNotices` does after every change. */
function publishList(store: NoticesStore): void {
  store.publish(HOME_TOPIC, EventTypeNoticeDismissed, { notices: store.notices });
}

/**
 * One notice call: keep one card awake, sleep one card now, keep every card the notice names awake,
 * or sleep them all now. It answers how many cards it changed, the way the daemon's own handler does.
 * An action name this daemon has no meaning for is an invalid argument, because nothing about the
 * notice or the cards is wrong.
 */
function noticeAction(store: NoticesStore, id: string, request: FakeRequest): Response {
  const body = bodyOf(request);
  const action = asText(body.action);
  const cardId = asText(body.cardId);
  switch (action) {
    case NoticeActionKeepAwake:
    case NoticeActionSleepNow:
      if (!cardId) {
        return errorAnswer(
          STATUS.badRequest,
          "invalid_argument",
          "A card is needed for that notice call.",
        );
      }
      // Both take one card off its notice. What the daemon does to the card itself - hold it off
      // the idle timer, or put its session to sleep - is not a thing this store models: a test that
      // wants the card's own change follows the card routes, and one that wants a refusal asks for
      // it with `refuseNext`.
      takeCard(store, cardId);
      return jsonAnswer({ cards: 1 } satisfies NoticeActionResult);
    case NoticeActionKeepAll:
    case NoticeActionSleepAll:
      // The two whole-notice calls answer the cards the notice named, so a notice that is already
      // gone answers zero rather than an error.
      return jsonAnswer({ cards: takeNotice(store, id) } satisfies NoticeActionResult);
    default:
      return errorAnswer(
        STATUS.badRequest,
        "invalid_argument",
        `There is no ${JSON.stringify(action)} action on a notice, so nothing was changed. A notice` +
          " can be kept awake, slept now, kept all awake, or slept all now.",
      );
  }
}

/**
 * Clears a notice without changing any card. The daemon holds each of its cards off the idle timer,
 * which this store does not model; a notice that is already gone is not an error and changes
 * nothing, so it announces nothing either.
 */
function dismiss(store: NoticesStore, id: string): Response {
  if (!store.notices.some((notice) => notice.id === id)) return emptyAnswer();
  store.notices = store.notices.filter((notice) => notice.id !== id);
  publishList(store);
  return emptyAnswer();
}

/** Takes one card off whichever notice names it, dropping a notice left with no cards. */
function takeCard(store: NoticesStore, cardId: string): void {
  let dropped = false;
  const kept: Notice[] = [];
  for (const notice of store.notices) {
    if (!(notice.cards ?? []).includes(cardId)) {
      kept.push(notice);
      continue;
    }
    const cards = (notice.cards ?? []).filter((one) => one !== cardId);
    dropped = true;
    if (cards.length > 0) kept.push({ ...notice, cards });
  }
  if (!dropped) return;
  store.notices = kept;
  publishList(store);
}

/** Removes the notice named by id and answers the cards it named, or 0 when there is none. */
function takeNotice(store: NoticesStore, id: string): number {
  if (id === "" || id.length > MAX_ECHOED_ID_BYTES) return 0;
  const found = store.notices.find((notice) => notice.id === id);
  if (!found) return 0;
  store.notices = store.notices.filter((notice) => notice !== found);
  publishList(store);
  return (found.cards ?? []).length;
}
