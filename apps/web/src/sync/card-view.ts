import type { ClientFrame, TerminalKey, Event as WireEvent } from "@marshal/protocol";
import { EventTypeSessionTerminalOutput } from "@marshal/protocol";
import { batch, createEffect, createSignal } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { ChangeInFlightError } from "~/data/optimistic";
import { isDaemon } from "~/data/sections";
import {
  buildTerminalInput,
  buildTerminalResize,
  buildTerminalSnapshot,
  type Frame,
} from "~/data/stream-frames";
import type { CardKey } from "~/mock/card-key";
import { type Ctx, sectionsOf } from "~/mock/context";
import { toast } from "~/mock/engine";
import { card as cardOf } from "~/mock/selectors";
import type { Mode } from "~/mock/types";

/*
 * S9's own write path (`switchView`, the daemon-backed `setMode`) and its read path (a card's
 * terminal channel, docs/architecture.md 4.2, 4.3, and the "terminal channel" paragraph of 11.2):
 * the chat/terminal switch itself, sending what a person types or presses into a real terminal, and
 * turning the daemon's raw, escape-coded bytes into the plain text `TerminalView.tsx` draws.
 *
 * This is deliberately not a terminal emulator (no cursor positioning, no alternate screens, no
 * `@xterm/xterm`): the owner decided a bundle-budget risk that could not be measured without a
 * build was not worth it for this pass. What is here instead is the daemon's output, decoded and
 * stripped of escape sequences, in the same scrolling block of plain text the mock always drew.
 */

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

/** The default size sent for a card's terminal (docs/architecture.md 4.3: "the default is 120 by
 * 32"), the same as `daemon/testdata/golden/terminal-screen.json`. Nothing here measures the
 * view's own box; a later pass can send a real size and this default stays the fallback. */
const DEFAULT_COLS = 120;
const DEFAULT_ROWS = 32;

/** How much decoded text a card's terminal buffer keeps, so a chatty program cannot grow it
 * forever in one long-lived tab. Bounding the decoded text (not the raw bytes) is enough: the
 * persistent `TextDecoder` below already carries any split multi-byte character across pieces. */
const BYTES_PER_KIB = 1024;
const MAX_TEXT_KIB = 64;
const MAX_TEXT_LENGTH = MAX_TEXT_KIB * BYTES_PER_KIB;

const viewKey = (id: CardKey): string => `view:${id}`;

/** The part of the event stream a card's terminal needs: sending its three client messages. The
 * subscribe/unsubscribe of the card's own topic is `card-session.ts`'s `followOpenCard`, already
 * running whenever this does (see the gate below), so it is not repeated here. */
export interface Stream {
  sendTerminal(frame: ClientFrame): void;
}

/**
 * The daemon-backed replacement for the mock's `setMode` (`mock/actions/navigation.ts`): switches
 * the open card between the chat view and the terminal view through `POST /v1/cards/{id}/view`.
 * Asking for the view the card is already in, or while a switch is already in flight, is a no-op,
 * the same short-circuit the mock has, so as to not send a request nothing needs. While the answer
 * is in flight it sets `ctx.S.switching` to the target mode exactly as the mock's timeout-based
 * version does, so `ChatTab.tsx`'s `ModeBar`/`SwitchingBody` need no change at all. On success it
 * sets `ctx.S.mode` to the answer's own mode and clears `switching`; on any failure `ctx.optimistic`
 * has already rolled `switching` back and shown the daemon's own sentence as a toast (the same
 * pattern every other refusal in this app uses), so nothing more is done here except a second toast
 * for the one failure `ctx.optimistic` does not itself show: two changes asked for at once.
 *
 * The caller (`mock/marshal.ts`) only reaches this for a card that is the daemon's; a mock-only
 * card is routed to the mock's own `setMode` instead, so this never has to guess a fallback.
 */
export async function switchView(ctx: Ctx, id: CardKey, mode: Mode): Promise<void> {
  const { S } = ctx;
  if (mode === S.mode || S.switching) return;
  const card = cardOf(ctx, id);
  const api = ctx.env.data?.api;
  if (!card?.daemonId) return;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return;
  }
  try {
    const view = await ctx.optimistic({
      key: viewKey(id),
      apply: () => {
        S.switching = mode;
      },
      request: () => api.setCardView(card.daemonId as string, { mode }),
      rollback: () => {
        S.switching = false;
      },
    });
    // The store's own card, not just `S.mode`: `followOpenCardTerminal`'s effect reads
    // `card.viewMode` to decide whether a reopened (or reconnected) card shows the terminal view,
    // and the daemon may answer here before (or instead of) a `card.updated` that says the same
    // thing — a chat process that goes straight back to awake with no visible change (section 4.3's
    // `swap`) publishes none at all. Left unwritten, the very next tick could read the card's own
    // (still old) `viewMode` and set `S.mode` straight back to what it was.
    batch(() => {
      S.mode = view.mode;
      S.switching = false;
      card.viewMode = view.mode;
    });
  } catch (error) {
    // `ctx.optimistic` has already rolled `switching` back and toasted the daemon's own sentence
    // for a refusal; the one case it cannot toast is a second switch asked for while the first is
    // still running, which throws before `optimistic` ever starts (`card-hold.ts`'s `ask` does the
    // same).
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
  }
}

/**
 * Strips ANSI escape sequences from decoded terminal output, for the plain-text render this pass
 * uses instead of a real terminal emulator: the common CSI sequences (`\x1b[...` — cursor moves,
 * colors, clears), OSC sequences (`\x1b]...\x07` — window titles), and any other escape sequence it
 * does not know by name, one byte at a time. A carriage return on its own is left alone: it is not
 * an escape sequence, and stripping it would make a program's own line endings unreadable. This is
 * a small, deliberate simplification (no cursor positioning), not a terminal emulator.
 */
// Matching the ESC/BEL bytes a terminal's own escape sequences use is the entire point of these,
// so each is its own named constant with the control-character lint turned off for its one line.
// biome-ignore lint/suspicious/noControlCharactersInRegex: a CSI sequence, ESC "[" ... a letter.
const CSI_SEQUENCE = /\x1b\[[0-9;?]*[A-Za-z]/g;
// biome-ignore lint/suspicious/noControlCharactersInRegex: an OSC sequence, ESC "]" ... BEL (or ST).
const OSC_SEQUENCE = /\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?/g;
// biome-ignore lint/suspicious/noControlCharactersInRegex: any other two-byte escape sequence.
const OTHER_ESCAPE = /\x1b./g;
// biome-ignore lint/suspicious/noControlCharactersInRegex: a lone ESC with nothing after it.
const LONE_ESCAPE = /\x1b/g;

export function stripAnsi(text: string): string {
  return text
    .replace(CSI_SEQUENCE, "")
    .replace(OSC_SEQUENCE, "")
    .replace(OTHER_ESCAPE, "")
    .replace(LONE_ESCAPE, "");
}

/** One card's terminal, kept per store (`Ctx`) so two test stores never share one. */
interface TermEntry {
  getText: () => string;
  setText: (text: string) => void;
  /** A persistent decoder, so a piece that ends mid-character is finished by the next one
   * (`{ stream: true }`), instead of decoding each piece alone. */
  decoder: TextDecoder;
  /** The decoded text since the last screen, before stripping: what is shown is stripped from all
   * of this each time, never piece by piece, so a CSI sequence split across two pieces is never
   * shown half-stripped. */
  decoded: string;
  /** The sequence number of the newest output event already included (a fresh screen's own
   * `throughSeq`), or +Infinity before any screen has arrived, so a stray output event never lands
   * ahead of the screen it belongs after. */
  throughSeq: number;
}

const TERMS = new WeakMap<Ctx, Map<CardKey, TermEntry>>();

function termsOf(ctx: Ctx): Map<CardKey, TermEntry> {
  const found = TERMS.get(ctx);
  if (found) return found;
  const made = new Map<CardKey, TermEntry>();
  TERMS.set(ctx, made);
  return made;
}

function entryFor(ctx: Ctx, key: CardKey): TermEntry {
  const terms = termsOf(ctx);
  const found = terms.get(key);
  if (found) return found;
  const [getText, setText] = createSignal("");
  const made: TermEntry = {
    getText,
    setText,
    decoder: new TextDecoder("utf-8", { fatal: false }),
    decoded: "",
    throughSeq: Number.POSITIVE_INFINITY,
  };
  terms.set(key, made);
  return made;
}

/** The decoded, escape-stripped text of a card's terminal, growing as output arrives. Empty for a
 * card whose terminal has sent nothing yet, or one nothing has ever applied to. */
export function terminalTextOf(ctx: Ctx, key: CardKey): string {
  return entryFor(ctx, key).getText();
}

/** Reads a piece of base64 the daemon sent. Malformed base64 (which the daemon never sends, but a
 * test double or a future bug might) decodes to nothing rather than throwing. */
function decodeBase64(data: string): Uint8Array {
  try {
    const binary = atob(data);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
    return bytes;
  } catch {
    return new Uint8Array(0);
  }
}

/** Appends a piece of base64 output to a card's terminal, decoding it with the entry's own
 * persistent decoder, bounding the buffer, and stripping escape sequences from the whole of it. */
function appendOutput(entry: TermEntry, data: string): void {
  if (!data) return;
  const piece = entry.decoder.decode(decodeBase64(data), { stream: true });
  const combined = entry.decoded + piece;
  entry.decoded = combined.length > MAX_TEXT_LENGTH ? combined.slice(-MAX_TEXT_LENGTH) : combined;
  entry.setText(stripAnsi(entry.decoded));
}

/** The card a wire terminal message names, by the daemon's own id, the same lookup
 * `card-session.ts`'s `cardKeyOf` does (a card's events name it by its opaque id, not its key). */
function cardKeyOf(ctx: Ctx, daemonId: string): CardKey | null {
  return ctx.S.cards.find((one) => one.daemonId === daemonId)?.id ?? null;
}

/**
 * Applies a `terminal.screen` or `terminal.refused` frame (`data/event-stream.ts`'s
 * `onTerminalFrame`, wired in `sync/index.ts`). A screen replaces the buffer outright — it is the
 * terminal's whole recent output, not a piece to append — with a fresh decoder, since it is a fresh
 * window and any state left over from before it belongs to output that is gone. A refusal shows
 * the daemon's own sentence as a toast, the same pattern `card-hold.ts` uses for its own refusals;
 * the connection stays open, so nothing else changes.
 */
export function applyTerminalFrame(ctx: Ctx, frame: Frame, daemonId: string): void {
  const key = cardKeyOf(ctx, daemonId);
  if (!key) return;
  if (frame.kind === "terminal.screen") {
    const entry = entryFor(ctx, key);
    entry.decoder = new TextDecoder("utf-8", { fatal: false });
    entry.decoded = "";
    appendOutput(entry, frame.frame.data);
    entry.throughSeq = frame.frame.throughSeq;
    return;
  }
  if (frame.kind === "terminal.refused") toast(ctx, frame.frame.error.message);
}

/**
 * Applies one `session.terminal_output` event (docs/architecture.md 11.2): live-only, so it is
 * never replayed, and applied only when its number is higher than the last screen's `throughSeq` —
 * anything at or below that is already part of the screen that was painted. An event for a card
 * whose terminal has not had a screen yet (`throughSeq` still +Infinity) is dropped, on the
 * assumption its screen is on the way (the daemon flushes waiting events before it answers a
 * snapshot, so nothing between the two is ever missed).
 */
export function applyTerminalOutputEvent(ctx: Ctx, event: WireEvent): void {
  if (event.type !== EventTypeSessionTerminalOutput) return;
  const data =
    event.data && typeof event.data === "object" ? (event.data as Record<string, unknown>) : null;
  const daemonId = data && typeof data.cardId === "string" ? data.cardId : "";
  const key = daemonId ? cardKeyOf(ctx, daemonId) : null;
  if (!key) return;
  const entry = entryFor(ctx, key);
  if (event.seq <= entry.throughSeq) return;
  const bytes = data && typeof data.data === "string" ? data.data : "";
  appendOutput(entry, bytes);
  entry.throughSeq = event.seq;
}

/** The part of the store a card's terminal needs to find the open one. */
function openCardOf(ctx: Ctx) {
  const key = ctx.S.openId;
  return key ? ctx.S.cards.find((one) => one.id === key) : undefined;
}

/**
 * Keeps a card's terminal current while it is the one open and in the terminal view: it asks for a
 * fresh screen (and sends the view's size, the fixed default above) whenever the open card, or its
 * view, changes into showing a real terminal — the same reactive shape `followOpenCard` uses for
 * the card's own topic, which is already subscribed whenever this runs (see the gate below), so
 * nothing here subscribes or unsubscribes anything. It also keeps `ctx.S.mode` in step with the
 * card's own stored `viewMode` (`data/mappers/card.ts`): a reopened card, or one whose session left
 * the terminal view on its own, shows the view it is actually in, the way a page reload would,
 * while a switch already in flight (`ctx.S.switching`) is left alone to finish on its own.
 *
 * It returns a function that asks again: `sync/index.ts` calls it after a resync and after a
 * reconnection, the same way it calls `card-session.ts`'s `rereadChat`, because terminal output is
 * never replayed (docs/architecture.md 11.2) — what the screen missed while the connection was away
 * is gone, and only a fresh snapshot puts it right.
 */
export function followOpenCardTerminal(ctx: Ctx, _api: ApiClient, stream: Stream): () => void {
  if (!isDaemon("S9", sectionsOf(ctx.env))) return () => undefined;
  let sentFor: string | null = null;
  // Deferred a microtask, so a resend that runs from the stream's own "the connection just opened"
  // notification (`data/connection.ts`'s `onReconnected`, called synchronously from inside
  // `event-stream.ts`'s `opened()`) never race the Hello that same notification is sent ahead of:
  // `opened()` calls `setState("open")` (which is what fires `onReconnected`) *before* it calls its
  // own `sendHello()`, so asking for a card's terminal in the same tick would put a `terminal.*`
  // message on the wire before the Hello that subscribes its topic — which the daemon answers the
  // way it answers any message for a topic the connection does not (yet) follow: an error frame,
  // and the connection ends. A microtask runs after the rest of that synchronous call stack,
  // `sendHello()` included, so the Hello is always first.
  const ask = (daemonId: string): void => {
    queueMicrotask(() => {
      stream.sendTerminal(buildTerminalResize(daemonId, DEFAULT_COLS, DEFAULT_ROWS));
      stream.sendTerminal(buildTerminalSnapshot(daemonId));
    });
  };
  createEffect(() => {
    const card = openCardOf(ctx);
    if (card?.daemonId && card.viewMode && !ctx.S.switching && ctx.S.mode !== card.viewMode) {
      ctx.S.mode = card.viewMode;
    }
    const inTerminal = ctx.S.mode === "terminal" && !ctx.S.switching;
    const wanted = card?.daemonId && inTerminal ? card.daemonId : null;
    if (wanted === sentFor) return;
    sentFor = wanted;
    if (wanted) ask(wanted);
  });
  return () => {
    if (sentFor) ask(sentFor);
  };
}

/**
 * Sends the person's own typed command into a card's real terminal, with the trailing line end the
 * client adds (`daemon/testdata/golden/terminal-input.json`'s `data` is `"ls -la\r"`, so the client
 * appends `\r`, not the daemon). Unlike a chat message, the text is sent as it is: not trimmed, and
 * an empty field is still sent (as a bare `\r`), because the key bar has no Enter button, and a
 * bare Enter is the only way this view answers a program's own "press Enter" prompt.
 */
export function sendTerminalText(ctx: Ctx, id: CardKey, text: string): void {
  const card = cardOf(ctx, id);
  const stream = ctx.env.data?.stream;
  if (!card?.daemonId || !stream) return;
  stream.sendTerminal(buildTerminalInput(card.daemonId, { data: `${text}\r` }));
}

/** Sends one of the key bar's presses into a card's real terminal, as the named wire key it maps
 * to (`TerminalView.tsx` does the mapping, since it is the one that knows the bar's own key names). */
export function sendTerminalKey(ctx: Ctx, id: CardKey, key: TerminalKey): void {
  const card = cardOf(ctx, id);
  const stream = ctx.env.data?.stream;
  if (!card?.daemonId || !stream) return;
  stream.sendTerminal(buildTerminalInput(card.daemonId, { key }));
}
