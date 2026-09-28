/**
 * The actions of one card in the fake daemon (section S7c): the session's moves (start, stop,
 * resume, pause, unpause, sleep, wake), a move between columns, a fork, the pin, and the reads that
 * answer one card's chat and activity. The refusal sentences and reasons are the daemon's own, word
 * for word, so a test can drive the same refusal a person sees.
 */
import type {
  ActivityItem,
  BypassRequest,
  ChatMessageDetail,
  CiRun,
  MoveCardRequest,
  SaveNoteRequest,
  SessionState,
  SimulateCIFailureRequest,
  SimulateCIFailureResult,
  Card as WireCard,
  Note as WireNote,
} from "@marshal/protocol";
import { emptyAnswer, errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";
import {
  BOARD_COLUMNS,
  BYPASS,
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

/** The commit the real mode reports having pushed. It is not a real object; the app never reads it back. */
const MARKED_COMMIT = "4f0b1c2d3e4a5b6c7d8e9f0a1b2c3d4e5f607182";

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

/**
 * The bypass routes (section S7b, docs/backend-checklist.md B3.2): POST turns bypass on for a card
 * and DELETE turns it off. Turning it on is the one card setting that is not a plain field change,
 * so the POST carries the acknowledgement a person gave and a body that does not acknowledge it is
 * refused, as is a card of a project whose settings lock bypass. Turning it off is always allowed
 * and leaves the card in full auto, the way the daemon's own `setBypass` does. Both answer the card
 * as it now is, with no body when nothing changed.
 */
export function cardBypass(
  store: CardStore,
  card: WireCard,
  request: FakeRequest,
  method: string,
  locked: boolean,
): Response | undefined {
  if (method === "POST") return grantBypass(store, card, request, locked);
  if (method === "DELETE") return clearBypass(store, card);
  return undefined;
}

/** POST /v1/cards/{id}/bypass: the acknowledgement first, then the project's lock, then the write. */
function grantBypass(
  store: CardStore,
  card: WireCard,
  request: FakeRequest,
  locked: boolean,
): Response {
  const body = JSON.parse(request.body ?? "{}") as BypassRequest;
  if (body.acknowledged !== true) {
    return refused("unacknowledged", BYPASS.unacknowledged, {
      permissionMode: "bypass",
      cardId: card.id,
    });
  }
  if (locked) {
    return refused("locked", BYPASS.locked, { cardId: card.id, projectId: card.projectId });
  }
  return setPermissionMode(store, card, "bypass");
}

/** DELETE /v1/cards/{id}/bypass: the card is left in full auto, whatever mode it held before. */
function clearBypass(store: CardStore, card: WireCard): Response {
  return setPermissionMode(store, card, "full-auto");
}

/** Writes a card's permission mode and announces it, unless it already holds that mode. */
function setPermissionMode(store: CardStore, card: WireCard, mode: string): Response {
  if (card.permissionMode !== mode) {
    card.permissionMode = mode as WireCard["permissionMode"];
    card.updatedAt = store.now();
    publishCard(store, card, "card.updated");
  }
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
  if (action === "note" && !extra) return jsonAnswer(cardNote(store, card));
  return undefined;
}

/**
 * A card's note (section S14, task 7.12): what is stored, or the placeholder the daemon would
 * start one from - its title, its goal, and a link to its project - with no save time, matching
 * `internal/memory.placeholderNote`. A read never writes: no entry is made here.
 */
function cardNote(store: CardStore, card: WireCard): WireNote {
  const existing = store.notes[card.id];
  if (existing) return existing;
  return {
    cardId: card.id,
    projectId: card.projectId,
    path: noteRelPath(card),
    body: `# ${card.title}\n\nGoal: ${card.title.toLowerCase()}.\n\nLinks\n[[${card.projectId}]]\n`,
    author: "person",
    updatedAt: null,
  };
}

/**
 * Writes a card's note: the file (in the real daemon) and the row, in one call. This fake keeps
 * only the row - there is no vault on disk in a test - but the shape it answers, and the path it
 * computes, follow `internal/memory`'s `noteRelPath` exactly, so a mapper test reading this route's
 * answer sees what the real one would.
 */
export function saveCardNoteRoute(
  store: CardStore,
  card: WireCard,
  request: FakeRequest,
): Response {
  const body = JSON.parse(request.body ?? "{}") as SaveNoteRequest;
  const note: WireNote = {
    cardId: card.id,
    projectId: card.projectId,
    path: noteRelPath(card),
    body: body.body,
    author: "person",
    updatedAt: store.now(),
  };
  store.notes[card.id] = note;
  return jsonAnswer(note);
}

/**
 * Where a card's note lives in the vault, relative to its root: `<project>/cards/<n>-<title>.md`.
 * Mirrors `internal/memory`'s `noteRelPath`/`noteSlug` byte for byte, so a fake daemon answers the
 * same path a real one would for the same card.
 */
function noteRelPath(card: WireCard): string {
  return `${card.projectId}/cards/${card.number}-${noteSlug(card.title)}.md`;
}

const NOTE_SLUG_MAX = 60;

/**
 * The readable part of a note's file name: lower case, hyphenated, ASCII letters and digits only.
 * Ports `internal/memory`'s `noteSlug` byte for byte: any run of characters that is not a lower-case
 * ASCII letter or digit becomes exactly one hyphen (never two, and never one at the very start or
 * end), the same way Go's version only ever inserts a hyphen lazily, right before the next letter or
 * digit it keeps. `a/b` slugs to `a-b`, not `ab` - the punctuation is a separator, not nothing.
 */
function noteSlug(title: string): string {
  const collapsed = title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  const cut = collapsed.slice(0, NOTE_SLUG_MAX).replace(/-+$/, "");
  return cut || "note";
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
    "ci-failure": () => simulateCIFailure(card, request),
  };
  return Object.hasOwn(actions, action) ? actions[action]?.() : undefined;
}

/**
 * A simulated CI failure (N28, B6.4). The fake daemon answers the wire shape of both modes and the
 * body is what a test reads; the run it names is the golden one, renamed for the card it was asked
 * about, and a mode Marshal does not know is refused the way the real route refuses it. The card's
 * own CI is left as it is: the real daemon says what changed through its own `card.updated` and
 * `ci.updated` events, which a test publishes for itself.
 */
function simulateCIFailure(card: WireCard, request: FakeRequest): Response {
  const body = JSON.parse(request.body ?? "{}") as SimulateCIFailureRequest;
  if (body.mode !== "synthetic" && body.mode !== "real") {
    return errorAnswer(STATUS.badRequest, "invalid_argument", "That is not a mode Marshal knows.");
  }
  const run: CiRun = {
    ...golden<CiRun>("ci-run"),
    cardId: card.id,
    projectId: card.projectId,
    branch: card.branch,
  };
  return jsonAnswer({
    cardId: card.id,
    mode: body.mode,
    run,
    fixStarted: true,
    commit: body.mode === "real" ? MARKED_COMMIT : "",
  } satisfies SimulateCIFailureResult);
}
