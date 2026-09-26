import {
  type EventBatch,
  EventTypeValues,
  FrameTypeError,
  FrameTypeEvents,
  FrameTypeHello,
  FrameTypeResync,
  FrameTypeTerminalInput,
  FrameTypeTerminalRefused,
  FrameTypeTerminalResize,
  FrameTypeTerminalScreen,
  FrameTypeTerminalSnapshot,
  type Hello,
  type Resync,
  ResyncReasonValues,
  type TerminalInput,
  type TerminalKey,
  type TerminalRefusal,
  type TerminalResize,
  type TerminalScreen,
  type TerminalSnapshotRequest,
  type Topic,
  type Error as WireError,
  type Event as WireEvent,
} from "@marshal/protocol";
import { readWireError } from "./api-error";
import { isRecord } from "./guards";

/** A frame from the daemon that the stream understood. */
export type Frame =
  | { kind: "events"; frame: EventBatch; dropped: number }
  | { kind: "resync"; frame: Resync }
  | { kind: "error"; error: WireError }
  | { kind: "terminal.screen"; frame: TerminalScreen }
  | { kind: "terminal.refused"; frame: TerminalRefusal };

function readEvent(value: unknown): WireEvent | null {
  if (!isRecord(value)) return null;
  const { seq, topic, type, at, data } = value;
  const known = EventTypeValues.find((candidate) => candidate === type);
  if (typeof seq !== "number" || typeof topic !== "string" || typeof at !== "string") return null;
  return known === undefined ? null : { seq, topic, type: known, at, data };
}

function readEvents(body: Record<string, unknown>): Frame | null {
  if (typeof body.epoch !== "string" || !Array.isArray(body.events)) return null;
  const events = body.events.map(readEvent).filter((event) => event !== null);
  // An event that is not readable, such as a type this app does not know yet, is left out. The
  // rest of the frame is used: a gap in the numbers is normal, so nothing else is lost.
  return {
    kind: "events",
    frame: { type: FrameTypeEvents, epoch: body.epoch, events },
    dropped: body.events.length - events.length,
  };
}

function readResync(body: Record<string, unknown>): Frame | null {
  const { epoch, seq, reason } = body;
  const known = ResyncReasonValues.find((candidate) => candidate === reason);
  if (typeof epoch !== "string" || typeof seq !== "number" || known === undefined) return null;
  return { kind: "resync", frame: { type: FrameTypeResync, epoch, reason: known, seq } };
}

/**
 * Reads a `terminal.screen` frame (a card's terminal channel, docs/architecture.md 11.2): the
 * answer to a `terminal.snapshot` request. `data` is checked by type only, never truthiness, so an
 * empty string (a terminal that has printed nothing yet, `throughSeq` 0) is still a valid frame.
 */
function readTerminalScreen(body: Record<string, unknown>): Frame | null {
  const { cardId, cols, rows, throughSeq, data } = body;
  if (
    typeof cardId !== "string" ||
    typeof cols !== "number" ||
    typeof rows !== "number" ||
    typeof throughSeq !== "number" ||
    typeof data !== "string"
  ) {
    return null;
  }
  return {
    kind: "terminal.screen",
    frame: { type: FrameTypeTerminalScreen, cardId, cols, rows, throughSeq, data },
  };
}

/** Reads a `terminal.refused` frame: a terminal message that could not be done right now. The
 * connection stays open (unlike an `error` frame), so the app can act on it and keep going. */
function readTerminalRefused(body: Record<string, unknown>): Frame | null {
  const { cardId } = body;
  if (typeof cardId !== "string") return null;
  const error = readWireError(body.error);
  if (!error) return null;
  return { kind: "terminal.refused", frame: { type: FrameTypeTerminalRefused, cardId, error } };
}

/** Reads one message from the daemon. It returns null for anything it does not understand. */
export function parseFrame(data: unknown): Frame | null {
  if (typeof data !== "string") return null;
  let body: unknown;
  try {
    body = JSON.parse(data);
  } catch {
    return null;
  }
  if (!isRecord(body)) return null;
  switch (body.type) {
    case FrameTypeEvents:
      return readEvents(body);
    case FrameTypeResync:
      return readResync(body);
    case FrameTypeError: {
      const error = readWireError(body.error);
      return error ? { kind: "error", error } : null;
    }
    case FrameTypeTerminalScreen:
      return readTerminalScreen(body);
    case FrameTypeTerminalRefused:
      return readTerminalRefused(body);
    default:
      return null;
  }
}

/** The first message of a connection, and the one that changes the topics. */
export function buildHello(topics: Iterable<Topic>, sinceSeq: number, epoch: string): Hello {
  return { type: FrameTypeHello, subscribe: [...topics], sinceSeq, epoch };
}

/**
 * What the person typed into a card's terminal (docs/architecture.md 11.2): exactly one of `data`
 * (text, with any line end the caller wants already in it) or `key` (a named key the phone's
 * keyboard lacks) is ever sent, which this signature enforces at the call site.
 */
export function buildTerminalInput(
  cardId: string,
  input: { data: string } | { key: TerminalKey },
): TerminalInput {
  return { type: FrameTypeTerminalInput, cardId, ...input };
}

/** The size of the view that draws a card's terminal, in character cells. */
export function buildTerminalResize(cardId: string, cols: number, rows: number): TerminalResize {
  return { type: FrameTypeTerminalResize, cardId, cols, rows };
}

/** Asks for a card's recent terminal screen: the answer is one `terminal.screen` frame. */
export function buildTerminalSnapshot(cardId: string): TerminalSnapshotRequest {
  return { type: FrameTypeTerminalSnapshot, cardId };
}

/**
 * The address of the event stream. With no base address it is the page's own host, which the dev
 * server forwards to the dev daemon. The token is never in it: it travels as a subprotocol.
 */
export function toEventsUrl(baseUrl: string, page: { protocol: string; host: string }): string {
  const base = baseUrl ? new URL(baseUrl) : page;
  return `${base.protocol === "https:" ? "wss:" : "ws:"}//${base.host}/v1/events`;
}
