/**
 * The actions of one card in the fake daemon (section S7c): the session's moves (start, stop,
 * resume, pause, unpause, sleep, wake), a move between columns, a fork, the pin, and the reads that
 * answer one card's chat and activity. The refusal sentences and reasons are the daemon's own, word
 * for word, so a test can drive the same refusal a person sees.
 */
import type {
  ActivityItem,
  ChatMessageDetail,
  MoveCardRequest,
  SessionState,
  Card as WireCard,
} from "@marshal/protocol";
import { emptyAnswer, errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import {
  BOARD_COLUMNS,
  type CardStore,
  DEFAULT_LIMIT,
  freshCardId,
  notFound,
  pageOf,
  publishCard,
  queryOf,
  refused,
  STATUS,
  wireCard,
} from "./fake-card-shared";
import { answerCardView } from "./fake-terminal";

/** How many trailing characters of a card's id its fake session id borrows. */
const ID_TAIL = 15;

function moveCard(store: CardStore, card: WireCard, request: FakeRequest): Response {
  const body = JSON.parse(request.body ?? "{}") as MoveCardRequest;
  if (!BOARD_COLUMNS.includes(body.state)) {
    return errorAnswer(STATUS.badRequest, "invalid_argument", "That is not a column of the board.");
  }
  // The two rules a test is most likely to meet, in the daemon's own words, so a component test
  // can drive the refusal a person sees.
  if (card.state === "done") {
    return refused("move_from_done", "A card that is done stays done.");
  }
  if (body.state === "done") {
    return refused("move_to_done", "Only a merged pull request moves a card to Done.");
  }
  // Architecture.md 6.1 rule 3: every manual move to Needs you is refused, because a card waits on
  // a person when an agent is waiting, not because somebody dragged it there. The request carries
  // no reason at all (see protocol.MoveCardRequest), so there is nothing to accept here.
  if (body.state === "needs") {
    return refused(
      "move_to_needs",
      "Cards move to Needs you by themselves when an agent is waiting on you.",
    );
  }
  const from = card.state;
  card.state = body.state;
  card.needsReason = null;
  card.updatedAt = store.now();
  publishCard(store, card, "card.moved", { from });
  return jsonAnswer(card);
}

function forkCard(store: CardStore, card: WireCard): Response {
  const numbers = store.cards.filter((c) => c.projectId === card.projectId).map((c) => c.number);
  const forked = wireCard({
    ...card,
    id: freshCardId(),
    number: Math.max(0, ...numbers) + 1,
    key: `${card.projectId}#${Math.max(0, ...numbers) + 1}`,
    title: `${card.title} (fork)`,
    state: "backlog",
    doingNow: "Starting from the latest checkpoint",
    createdAt: store.now(),
    updatedAt: store.now(),
  });
  store.cards.push(forked);
  publishCard(store, forked, "card.created");
  return jsonAnswer(forked, STATUS.created);
}

/**
 * The session states in which a process runs or is about to (`awakeSessionState` in the daemon's
 * hold.go): the ones a sleep can put to sleep.
 */
const HAS_PROCESS: readonly SessionState[] = ["starting", "awake", "working", "waking"];

/**
 * Moves a card's session to a state and says so the way the daemon does (`publishSessionState`):
 * `session.state_changed` on the card's own topic, then the card as it now is, with its new
 * `session`, as `card.updated` on the project's. The card's screens follow the second; the open
 * card hears the first a moment sooner.
 */
function setSession(store: CardStore, card: WireCard, state: SessionState): void {
  card.session = state;
  card.updatedAt = store.now();
  store.publish(`card:${card.id}`, "session.state_changed", {
    cardId: card.id,
    sessionId: `01M3SESSION${card.id.slice(-ID_TAIL)}`,
    state,
  });
  publishCard(store, card, "card.updated");
}

/** Brings a card's session back: it reads waking, then awake, and the card is working again. */
function resumeSession(store: CardStore, card: WireCard): void {
  setSession(store, card, "waking");
  if (card.state !== "working") {
    const from = card.state;
    card.state = "working";
    card.needsReason = null;
    publishCard(store, card, "card.moved", { from });
  }
  setSession(store, card, "awake");
}

/**
 * POST /v1/cards/{id}/start, as the daemon answers it: a card with no session gets one and is
 * working; a paused card is released; a sleeping or stopped session is resumed (`Manager.Start`
 * resumes the saved session, it never starts a second one). The card is answered as it now is.
 */
function startCard(store: CardStore, card: WireCard): Response {
  if (card.session === null) {
    card.state = card.permissionMode === "plan" ? "planning" : "working";
    setSession(store, card, "awake");
  } else if (card.session === "asleep" || card.session === "stopped") {
    card.paused = false;
    resumeSession(store, card);
  } else if (card.paused) {
    card.paused = false;
    card.updatedAt = store.now();
    publishCard(store, card, "card.updated");
  } else {
    card.updatedAt = store.now();
    publishCard(store, card, "card.updated");
  }
  return jsonAnswer(card);
}

/** POST /v1/cards/{id}/stop: the session ends, the card is answered with no body, and the change is said. */
function stopCard(store: CardStore, card: WireCard): Response {
  setSession(store, card, "stopped");
  return emptyAnswer();
}

/** POST /v1/cards/{id}/resume: the session comes back, and the answer has no body. */
function resumeCard(store: CardStore, card: WireCard): Response {
  resumeSession(store, card);
  return emptyAnswer();
}

/** POST /v1/cards/{id}/messages: the answer to a message arrives on the stream, so the route is empty. */
function sendMessage(): Response {
  return emptyAnswer();
}

/**
 * The session hold routes (section S7c, docs/architecture.md 5.1): pause, unpause, sleep, wake,
 * pin, and unpin. The refusal sentences and reasons are the daemon's own, word for word, so a test
 * can drive the same refusal a person sees. Sleep and wake answer with no body, the way the real
 * routes do; what changed is the card's session, and it is said the way the daemon says it (see
 * `setSession`).
 */
function pauseCard(store: CardStore, card: WireCard): Response {
  if (card.state !== "working") {
    return refused("pause_not_working", "Only working cards can be paused.");
  }
  card.paused = true;
  card.updatedAt = store.now();
  publishCard(store, card, "card.updated");
  return jsonAnswer(card);
}

function unpauseCard(store: CardStore, card: WireCard): Response {
  if (card.paused) {
    card.paused = false;
    card.updatedAt = store.now();
    publishCard(store, card, "card.updated");
  }
  return jsonAnswer(card);
}

/**
 * The rules of `Manager.Sleep`, in the daemon's order and with its sentences: a working card that is
 * not paused does not sleep, a card waiting on the person stays awake, and a session with no process
 * has nothing to put to sleep. A sleep changes the session and nothing else: the card keeps its
 * state, its pause, and its pin.
 */
function sleepCard(store: CardStore, card: WireCard): Response {
  if (card.state === "working" && !card.paused) {
    return refused("sleep_working", "Working cards don't sleep. Pause the card first.");
  }
  if (card.state === "needs") {
    return refused("sleep_needs_you", "This card is waiting on you, so it stays awake.");
  }
  if (!card.session || !HAS_PROCESS.includes(card.session)) {
    return refused("sleep_no_session", "This card has no awake session.");
  }
  setSession(store, card, "asleep");
  return emptyAnswer();
}

/**
 * The rules of `Manager.Wake`: a card that never had a session is not found, one whose session has
 * stopped cannot be woken (Start is the way back), and a session that is already awake is left
 * alone. A sleeping one reads waking, then awake, and the card is working again.
 */
function wakeCard(store: CardStore, card: WireCard): Response {
  if (!card.session) {
    return errorAnswer(
      STATUS.notFound,
      "not_found",
      "Marshal cannot find that session. It may have been removed.",
    );
  }
  if (card.session === "stopped") {
    return jsonAnswer(
      {
        error: {
          code: "refused",
          message: "This card's session has stopped and cannot be resumed.",
          details: { cardId: card.id },
        },
      },
      STATUS.refused,
    );
  }
  if (card.session === "asleep") resumeSession(store, card);
  return emptyAnswer();
}

function pinCard(store: CardStore, card: WireCard): Response {
  card.pinned = true;
  card.updatedAt = store.now();
  publishCard(store, card, "card.updated");
  return jsonAnswer(card);
}

function unpinCard(store: CardStore, card: WireCard): Response {
  card.pinned = false;
  card.updatedAt = store.now();
  publishCard(store, card, "card.updated");
  return jsonAnswer(card);
}

/** What a GET under one card answers: the card itself, its chat, its activity, or one message. */
export function cardRead(
  store: CardStore,
  card: WireCard,
  request: FakeRequest,
  action: string | undefined,
  extra: string | undefined,
): Response | undefined {
  if (!action) return extra ? undefined : jsonAnswer(card);
  const query = queryOf(request.url);
  const limit = Number(query.get("limit")) || DEFAULT_LIMIT;
  const cursor = query.get("cursor") ?? "";
  if (action === "messages") {
    return extra ? cardMessage(store, card.id, extra) : cardMessages(store, card.id, limit, cursor);
  }
  if (action === "activity") {
    return cardActivity(store, card.id, limit, cursor, query.get("kind") ?? null);
  }
  return undefined;
}

/**
 * A card's chat: its stored history newest first, one page at a time. Like the real daemon, a card
 * that is not there is a not_found and a page never carries a missing list.
 */
function cardMessages(store: CardStore, cardId: string, limit: number, cursor: string): Response {
  if (!store.cards.some((card) => card.id === cardId)) return notFound();
  const rows = store.history
    .filter((row) => row.cardId === cardId)
    .map((row) => row.message)
    .sort((a, b) => b.seq - a.seq);
  return jsonAnswer(pageOf(rows, limit, cursor, store.now()));
}

/** A card's activity: the history rows that draw one, newest first, optionally one kind. */
function cardActivity(
  store: CardStore,
  cardId: string,
  limit: number,
  cursor: string,
  kind: string | null,
): Response {
  if (!store.cards.some((card) => card.id === cardId)) return notFound();
  const rows = store.history
    .filter((row) => row.cardId === cardId && row.activity)
    .map((row) => row.activity as ActivityItem)
    .filter((item) => !kind || item.kind === kind)
    .sort((a, b) => b.seq - a.seq);
  return jsonAnswer(pageOf(rows, limit, cursor, store.now()));
}

/** One message in full, with the tool detail the page leaves out. */
function cardMessage(store: CardStore, cardId: string, messageId: string): Response {
  const row = store.history.find((one) => one.cardId === cardId && one.message.id === messageId);
  if (!row) return notFound();
  const detail: ChatMessageDetail = { message: row.message, tool: null, serverTime: store.now() };
  return jsonAnswer(detail);
}

/** The POST actions of one card, by the word that ends the route (`/v1/cards/{id}/<action>`). */
export function cardAction(
  store: CardStore,
  card: WireCard,
  request: FakeRequest,
  action: string,
): Response | undefined {
  const actions: Record<string, () => Response> = {
    move: () => moveCard(store, card, request),
    fork: () => forkCard(store, card),
    start: () => startCard(store, card),
    stop: () => stopCard(store, card),
    resume: () => resumeCard(store, card),
    messages: sendMessage,
    pause: () => pauseCard(store, card),
    unpause: () => unpauseCard(store, card),
    sleep: () => sleepCard(store, card),
    wake: () => wakeCard(store, card),
    pin: () => pinCard(store, card),
    unpin: () => unpinCard(store, card),
    view: () => answerCardView(store, card, request),
  };
  return Object.hasOwn(actions, action) ? actions[action]?.() : undefined;
}
