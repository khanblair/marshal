import type { Event as WireEvent } from "@marshal/protocol";
import {
  EventTypeSessionOutput,
  EventTypeSessionStateChanged,
  EventTypeSessionToolCall,
  type SessionState,
  SessionStateValues,
  SessionStateWorking,
} from "@marshal/protocol";
import { batch, createEffect } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { ApiError } from "~/data/api-error";
import { isRecord } from "~/data/guards";
import { sleepFlags } from "~/data/mappers/card";
import { isDaemon } from "~/data/sections";
import type { CardKey } from "~/mock/card-key";
import { type Ctx, sectionsOf } from "~/mock/context";
import { toast } from "~/mock/engine";
import { takeMid } from "~/mock/ids";
import type { Msg, ToolState } from "~/mock/types";
import { toStoredActivityList, toStoredMessages } from "./chat-mapper";

/** How many of the newest messages and activity entries a card shows when it is opened. */
const FIRST_PAGE = 50;

/**
 * The topic a card's own events arrive on. It is the card's own opaque id, which its routes take -
 * the store's key is not the same thing, and a topic named by a key would never deliver anything.
 */
const cardTopic = (daemonId: string): string => `card:${daemonId}`;

/**
 * Reads the open card's chat and its activity from the daemon, where the screens read them from the
 * store. Only the section that is on the daemon is read, so a card whose chat is still the mock's
 * keeps what the store already holds.
 *
 * A card that is deleted while it is open, or one the daemon has no history for, is simply empty.
 */
export async function readOpenCard(
  ctx: Ctx,
  api: ApiClient,
  card: { daemonId?: string | undefined },
  key: CardKey,
): Promise<void> {
  const daemonId = card.daemonId;
  if (!daemonId) return;
  const table = sectionsOf(ctx.env);
  const wantsChat = isDaemon("S8a", table);
  const wantsActivity = isDaemon("S10", table);
  const [messages, activity] = await Promise.all([
    wantsChat ? api.messages(daemonId, { limit: FIRST_PAGE }) : null,
    wantsActivity ? api.activity(daemonId, { limit: FIRST_PAGE }) : null,
  ]);
  // The card may have been closed, or another one opened, while the daemon was answering.
  if (ctx.S.openId !== key) return;
  if (messages) ctx.S.chat[key] = toStoredMessages(messages.items);
  if (activity) ctx.S.act[key] = toStoredActivityList(activity.items, ctx.clock.now());
}

/**
 * Keeps the event stream pointed at the card that is open: it subscribes to that card's own topic
 * when a card opens, unsubscribes when it closes or another card opens, and reads the card's chat
 * and activity once, at the moment it opens.
 *
 * A card's own events (its messages and its tool calls) arrive on `card:<id>`, so this belongs to
 * the card panel and not to the section-wide syncers, which follow every project.
 */
export function followOpenCard(ctx: Ctx, api: ApiClient, stream: Stream): void {
  const table = sectionsOf(ctx.env);
  // S7c also needs the open card's own topic: a sleep and a wake answer with no body (see
  // card-hold.ts), and the state they move to arrives as session.state_changed on it. The same
  // change reaches every card, open or not, as card.updated on the project topic (the card's
  // `session`, see data/mappers/card.ts), so this is the quicker of two ways to hear it and never
  // the only one. S9 needs it too: a card's terminal messages and its session.terminal_output
  // events are only ever accepted on a connection that follows the card's own topic
  // (docs/architecture.md 11.2), which `card-view.ts`'s `followOpenCardTerminal` relies on this
  // subscribing before it ever asks for a snapshot.
  if (
    !isDaemon("S8a", table) &&
    !isDaemon("S10", table) &&
    !isDaemon("S7c", table) &&
    !isDaemon("S9", table)
  ) {
    return;
  }
  let subscribed: string | null = null;
  createEffect(() => {
    const key = ctx.S.openId;
    const card = key ? ctx.S.cards.find((one) => one.id === key) : undefined;
    const wanted = card?.daemonId ? cardTopic(card.daemonId) : null;
    if (wanted === subscribed) return;
    if (subscribed) stream.unsubscribe([subscribed]);
    subscribed = wanted;
    if (!wanted || !card || !key) return;
    stream.subscribe([wanted]);
    void readOpenCard(ctx, api, card, key);
  });
}

/** The part of the event stream this needs: subscribing and unsubscribing to a topic. */
interface Stream {
  subscribe(topics: readonly string[]): void;
  unsubscribe(topics: readonly string[]): void;
}

/** The card a card-scoped event is about, found by the daemon's own id, which is what it carries. */
function cardKeyOf(ctx: Ctx, daemonId: string): CardKey | null {
  return ctx.S.cards.find((card) => card.daemonId === daemonId)?.id ?? null;
}

/**
 * Applies one event of a card's own topic to its chat, so a running card's words and tool calls
 * appear as they happen. The stored history is the same material, read back a page at a time; this
 * is the live half, and it is what makes a card that is working look like it is working.
 *
 * A `session.output` message chunk joins the agent message that is still streaming, or starts one.
 * A thought is drawn as the agent's own words too, which is what the screens do with it.
 */
export function applyCardSessionEvent(ctx: Ctx, event: WireEvent): void {
  const data = isRecord(event.data) ? event.data : null;
  const daemonId = data && typeof data.cardId === "string" ? data.cardId : "";
  const key = daemonId ? cardKeyOf(ctx, daemonId) : null;
  if (!key) return;
  if (event.type === EventTypeSessionStateChanged && data) {
    applySessionState(ctx, key, data);
    // The words of a turn are done once the session leaves it.
    if (data.state !== SessionStateWorking) endStreaming(ctx.S.chat[key] ?? []);
    return;
  }
  const chat = ctx.S.chat[key] ?? [];
  ctx.S.chat[key] = chat;
  if (event.type === EventTypeSessionOutput) {
    applyOutput(chat, ctx, data);
    return;
  }
  if (event.type === EventTypeSessionToolCall && data) applyToolCall(chat, ctx, data);
}

const isSessionState = (state: unknown): state is SessionState =>
  SessionStateValues.some((known) => known === state);

/**
 * A session's own state moved (section S7c): the card keeps it, and draws asleep and waking from it,
 * exactly as the mapper does for a card the daemon sent (`sleepFlags`), so this and the
 * `card.updated` that says the same thing on the project topic agree, whichever arrives first. A
 * state the app does not know is left alone.
 */
function applySessionState(ctx: Ctx, key: CardKey, data: Record<string, unknown>): void {
  const card = ctx.S.cards.find((one) => one.id === key);
  if (!card || !isSessionState(data.state)) return;
  const state = data.state;
  const flags = sleepFlags(state);
  // One change to the screens, so a card drawn from both is never drawn half way.
  batch(() => {
    if (card.session !== state) card.session = state;
    if (card.asleep !== flags.asleep) card.asleep = flags.asleep;
    if (card.waking !== flags.waking) card.waking = flags.waking;
  });
}

/**
 * Ends the agent's words that are still arriving: nothing marks a message finished on the wire, so
 * the screen does, when a tool call starts after them and when the session leaves its turn. Without
 * it the next answer would join the last one, and the message would stay busy for a screen reader.
 * Shared by a card's chat and a project chat's, which draw the same events.
 */
export function endStreaming(chat: Msg[]): void {
  for (const one of chat) {
    if (one.k === "agent" && one.streaming) one.streaming = false;
  }
}

/** A message chunk joins the agent message that is still streaming, or starts one. Also used by a project chat's own events. */
export function applyOutput(chat: Msg[], ctx: Ctx, data: Record<string, unknown> | null): void {
  const text = data && typeof data.text === "string" ? data.text : "";
  if (!text) return;
  const last = chat.at(-1);
  if (last?.k === "agent" && last.streaming) {
    last.text += text;
    return;
  }
  chat.push({ id: `s${takeMid(ctx.ids)}`, k: "agent", text, streaming: true });
}

/** A tool call appears as one line, and each update finds it by its own id and changes its state. */
export function applyToolCall(chat: Msg[], ctx: Ctx, data: Record<string, unknown>): void {
  if (!isRecord(data.toolCall)) return;
  const call = data.toolCall as { id?: unknown; title?: unknown; status?: unknown };
  const title = typeof call.title === "string" ? call.title : "";
  const state = typeof call.status === "string" ? call.status : "";
  const open = chat.find((one) => one.k === "tool" && (one.call ?? one.id) === call.id);
  if (open?.k === "tool") {
    if (title) open.action = title;
    if (state) open.st = toolState(state);
    return;
  }
  endStreaming(chat);
  const callId = typeof call.id === "string" && call.id ? call.id : `s${takeMid(ctx.ids)}`;
  chat.push({
    id: callId,
    call: callId,
    k: "tool",
    icon: "terminal",
    action: title,
    result: "",
    st: state ? toolState(state) : "running",
    detail: "",
    open: false,
  });
}

/**
 * How a tool call is going, from the status the wire says (`pending`, `in_progress`, `completed`, or
 * `failed`): only a call that has ended is drawn as ended, so a call that has just started is drawn
 * running, which is what the screens show while it works.
 */
const toolState = (status: string): ToolState => {
  if (status === "completed") return "ok";
  return status === "failed" ? "fail" : "running";
};

/**
 * Sends the person's own message into the card's session, and shows it at once: the daemon does not
 * publish the message back as an event, so nothing else would draw it until the card was reopened.
 * A message that the daemon refuses is taken back out of the chat, so the screen never shows words
 * that were never sent.
 */
export async function sendToCard(
  ctx: Ctx,
  api: ApiClient,
  key: CardKey,
  text: string,
): Promise<boolean> {
  const card = ctx.S.cards.find((one) => one.id === key);
  const said = text.trim();
  if (!card?.daemonId || !said) return false;
  const chat = ctx.S.chat[key] ?? [];
  ctx.S.chat[key] = chat;
  const shown = { id: `s${takeMid(ctx.ids)}`, k: "user" as const, text: said };
  chat.push(shown);
  try {
    await api.sendMessage(card.daemonId, { text: said });
    return true;
  } catch (error) {
    // By id: the array is the store's, and what it hands back for `shown` is not `shown` itself.
    ctx.S.chat[key] = (ctx.S.chat[key] ?? []).filter((one) => one.id !== shown.id);
    toast(
      ctx,
      error instanceof ApiError ? error.message : "Marshal could not send that. Try again.",
    );
    return false;
  }
}
