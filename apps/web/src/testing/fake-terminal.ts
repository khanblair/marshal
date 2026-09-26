/**
 * A card's terminal channel in the fake daemon (section S9, docs/architecture.md 4.2, 4.3, 11.2):
 * `POST /v1/cards/{id}/view`, and the three client messages on the event stream (`terminal.input`,
 * `terminal.resize`, `terminal.snapshot`), answered with `terminal.screen`/`terminal.refused` frames
 * and `session.terminal_output` events. It is a simplification of the daemon's own rules
 * (`daemon/internal/session/view.go`, `terminal.go`): the one rule it enforces itself is refusing a
 * view switch of a card with no agent (`view_no_agent`), because that is cheap and catches a real
 * mistake; every other view-switch refusal (a turn running, a held message, no terminal mode, a
 * switch already under way, a session that cannot resume) is left to `FakeDaemon.refuseNext`, the
 * daemon's own generic way to answer one call with any error a test names, since this app never
 * branches its own behaviour on which reason a switch was refused for — only on the sentence, which
 * `refuseNext` carries just as well.
 */
import type {
  TerminalInput,
  TerminalResize,
  Card as WireCard,
  Error as WireError,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import type { FakeWebSocket } from "~/data/testing/fake-web-socket";
import { type CardStore, publishCard, refused, STATUS } from "./fake-card-shared";

const NO_AGENT = () =>
  refused("view_no_agent", "This card has no agent running. Start the card first.");

const NOT_ACTIVE_ERROR: WireError = {
  code: "refused",
  message: "This card has no terminal running. Switch it to terminal view first.",
  details: { reason: "terminal_not_active" },
};
const BUSY_ERROR: WireError = {
  code: "refused",
  message: "The terminal is not keeping up with what you type. Wait a moment, then type again.",
  details: { reason: "terminal_busy" },
};

/** `POST /v1/cards/{id}/view` (section S9): the one rule this fake enforces on its own is
 * `view_no_agent` (a card with no session at all); every other refusal a test wants is driven
 * through `refuseNext`, since the client shows whichever sentence the daemon sends either way. */
export function answerCardView(store: CardStore, card: WireCard, request: FakeRequest): Response {
  const body = JSON.parse(request.body ?? "{}") as { mode?: string };
  if (body.mode !== "chat" && body.mode !== "terminal") {
    return errorAnswer(STATUS.badRequest, "invalid_argument", "That is not a view Marshal knows.");
  }
  if (!card.session) return NO_AGENT();
  card.viewMode = body.mode;
  card.updatedAt = store.now();
  publishCard(store, card, "card.updated");
  return jsonAnswer({
    cardId: card.id,
    mode: card.viewMode,
    session: card.session,
    serverTime: store.now(),
  });
}

/** One card's terminal, as the fake keeps it: whether it answers at all, whether it is reading
 * input, its size, the raw bytes it has "printed" so far (bounded like the real ring, 256 KiB), and
 * every message it received, for a test to assert on directly. */
interface FakeTerminal {
  active: boolean;
  busy: boolean;
  cols: number;
  rows: number;
  buffer: Uint8Array;
  /** The seq of the newest `session.terminal_output` this fake has published for this card. */
  throughSeq: number;
  inputs: TerminalInput[];
  resizes: TerminalResize[];
}

const DEFAULT_COLS = 120;
const DEFAULT_ROWS = 32;
const KIB = 1024;
const SCREEN_KIB = 256;
const MAX_SCREEN_BYTES = SCREEN_KIB * KIB;

function freshTerminal(): FakeTerminal {
  return {
    active: true,
    busy: false,
    cols: DEFAULT_COLS,
    rows: DEFAULT_ROWS,
    buffer: new Uint8Array(0),
    throughSeq: 0,
    inputs: [],
    resizes: [],
  };
}

/** What the terminal router needs from the fake daemon: publishing an event on the same seq
 * counter every other event uses, and its clock. */
export interface TerminalRouterDeps {
  publish: (topic: string, type: string, data: unknown) => void;
  /** The seq the event `publish` just sent was given, read right after calling it. */
  seqNow: () => number;
}

/** Everything a test can do to a card's terminal beside sending its wire messages, and the hook
 * `fakeSockets` calls for every message a client sends. */
export interface TerminalRouter {
  /** Reads every message a card's socket sent, and answers the three terminal ones the way the
   * daemon does, including ending the connection for a card whose topic it does not follow.
   * Ignores anything else (a `hello`, or a message this fake does not understand). */
  onSend(socket: FakeWebSocket, data: string): void;
  /** Publishes a piece of a card's output as `session.terminal_output`, live-only like the
   * daemon's own, and keeps it in the card's screen for the next snapshot. */
  emitOutput(cardId: string, text: string): void;
  /** Makes the next terminal message about a card answer `terminal.refused` (`terminal_not_active`),
   * or restores it, the way switching a card out of (or into) the terminal view would. */
  setActive(cardId: string, active: boolean): void;
  /** Makes `terminal.input` about a card answer `terminal.refused` (`terminal_busy`), or restores it. */
  setBusy(cardId: string, busy: boolean): void;
  /** Every `terminal.input` a card's terminal received, in order. */
  inputsOf(cardId: string): readonly TerminalInput[];
  /** Every `terminal.resize` a card's terminal received, in order. */
  resizesOf(cardId: string): readonly TerminalResize[];
}

/** Appends bytes to a terminal's own screen, bounded the way the real PTY's ring is. */
function append(term: FakeTerminal, bytes: Uint8Array): void {
  const combined = new Uint8Array(term.buffer.length + bytes.length);
  combined.set(term.buffer);
  combined.set(bytes, term.buffer.length);
  term.buffer = combined.length > MAX_SCREEN_BYTES ? combined.slice(-MAX_SCREEN_BYTES) : combined;
}

function toBase64(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

/** The topics a socket's latest `hello` asked to follow, or none if it never sent one. */
function followedTopics(socket: FakeWebSocket): Set<string> {
  const hellos = socket.hellos().filter((one) => one.type === "hello");
  const subscribe = hellos.at(-1)?.subscribe;
  return new Set(Array.isArray(subscribe) ? subscribe.filter((t) => typeof t === "string") : []);
}

const encoder = new TextEncoder();

/** Builds the fake daemon's terminal router: the `onSend` hook to give `fakeSockets`, and the ways
 * a test drives a card's terminal beside sending its wire messages. */
export function createTerminalRouter(deps: TerminalRouterDeps): TerminalRouter {
  const terminals = new Map<string, FakeTerminal>();
  const termOf = (cardId: string): FakeTerminal => {
    const found = terminals.get(cardId);
    if (found) return found;
    const made = freshTerminal();
    terminals.set(cardId, made);
    return made;
  };

  function refuse(socket: FakeWebSocket, cardId: string, error: WireError): void {
    socket.push({ type: "terminal.refused", cardId, error });
  }

  function endForBadTopic(socket: FakeWebSocket, cardId: string): void {
    // The real daemon's rule (`internal/api/stream_terminal.go`'s `checkFollows`): a terminal
    // message for a card whose topic the connection does not follow is a mistake in the client, so
    // it is answered like any other bad message on the stream, and the connection ends.
    socket.push({
      type: "error",
      error: {
        code: "invalid_argument",
        message:
          "Follow a card's topic before you use its terminal. Add the card to the topics of a hello first.",
        details: { topic: `card:${cardId}` },
      },
    });
    const BAD_MESSAGE = 1008;
    socket.drop(BAD_MESSAGE);
  }

  function onScreen(socket: FakeWebSocket, cardId: string): void {
    const term = termOf(cardId);
    if (!term.active) {
      refuse(socket, cardId, NOT_ACTIVE_ERROR);
      return;
    }
    socket.push({
      type: "terminal.screen",
      cardId,
      cols: term.cols,
      rows: term.rows,
      throughSeq: term.throughSeq,
      data: toBase64(term.buffer),
    });
  }

  function onInput(socket: FakeWebSocket, msg: TerminalInput): void {
    const term = termOf(msg.cardId);
    if (!term.active) {
      refuse(socket, msg.cardId, NOT_ACTIVE_ERROR);
      return;
    }
    if (term.busy) {
      refuse(socket, msg.cardId, BUSY_ERROR);
      return;
    }
    term.inputs.push(msg);
  }

  function onResize(socket: FakeWebSocket, msg: TerminalResize): void {
    const term = termOf(msg.cardId);
    if (!term.active) {
      refuse(socket, msg.cardId, NOT_ACTIVE_ERROR);
      return;
    }
    term.cols = msg.cols;
    term.rows = msg.rows;
    term.resizes.push(msg);
  }

  return {
    onSend(socket, data) {
      let msg: unknown;
      try {
        msg = JSON.parse(data);
      } catch {
        return;
      }
      if (typeof msg !== "object" || msg === null || !("type" in msg)) return;
      const { type } = msg as { type: unknown };
      if (type !== "terminal.snapshot" && type !== "terminal.input" && type !== "terminal.resize") {
        return;
      }
      const cardId = (msg as { cardId?: unknown }).cardId;
      if (typeof cardId !== "string") return;
      if (!followedTopics(socket).has(`card:${cardId}`)) {
        endForBadTopic(socket, cardId);
        return;
      }
      if (type === "terminal.snapshot") onScreen(socket, cardId);
      else if (type === "terminal.input") onInput(socket, msg as TerminalInput);
      else onResize(socket, msg as TerminalResize);
    },
    emitOutput(cardId, text) {
      const bytes = encoder.encode(text);
      const term = termOf(cardId);
      append(term, bytes);
      deps.publish(`card:${cardId}`, "session.terminal_output", { cardId, data: toBase64(bytes) });
      term.throughSeq = deps.seqNow();
    },
    setActive(cardId, active) {
      termOf(cardId).active = active;
    },
    setBusy(cardId, busy) {
      termOf(cardId).busy = busy;
    },
    inputsOf: (cardId) => termOf(cardId).inputs,
    resizesOf: (cardId) => termOf(cardId).resizes,
  };
}
