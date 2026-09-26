import {
  EventTypeSessionOutput,
  EventTypeSessionStateChanged,
  EventTypeSessionToolCall,
  SessionStateStopped,
  SessionStateWorking,
  type Event as WireEvent,
} from "@marshal/protocol";
import { batch, createEffect, untrack } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { ApiError } from "~/data/api-error";
import { isRecord } from "~/data/guards";
import { isDaemon } from "~/data/sections";
import { type Ctx, sectionsOf } from "~/mock/context";
import { toast } from "~/mock/engine";
import { takeMid } from "~/mock/ids";
import type { Chat, Msg } from "~/mock/types";
import { applyOutput, applyToolCall, endStreaming } from "./card-session";
import { toStoredMessages } from "./chat-mapper";

/*
 * The message side of a project chat (section S17, docs/backend-checklist.md B2.10): reading a chat's
 * history when it is opened, following its own topic while it is open, drawing the words and the tool
 * calls its agent sends, and sending the person's own message. A card's chat does the same in
 * `card-session.ts`, and this module is its twin: the wire carries the same messages, tool calls, and
 * session events for both, and the two screens draw them with the same mapper.
 *
 * A chat's events arrive on `chat:<id>` and name the chat (`chatId`, with `cardId` empty), so they
 * never reach a card. What the daemon says about a chat's list row (its name, when it last had a
 * message) arrives on the project's topic, and `chats.ts` draws that.
 */

/** How many of the newest messages a chat shows when it is opened. */
const FIRST_PAGE = 50;

const NOT_CONNECTED = "Marshal is not connected to its daemon.";
const READ_FAILED = "Marshal could not load this chat's messages. Try again.";
const SEND_FAILED = "Marshal could not send that. Try again.";

/** The topic a chat's own events arrive on. It is the chat's own opaque id, which its routes take. */
const chatTopic = (chatId: string): string => `chat:${chatId}`;

/** The daemon's own sentence when it wrote one, and a plain one otherwise. */
const sentence = (error: unknown, fallback: string): string =>
  error instanceof ApiError ? error.message : fallback;

/** The chat with this id, in whatever project it is. A chat's events name the chat and not its project. */
function chatOf(ctx: Ctx, chatId: string): Chat | undefined {
  for (const list of Object.values(ctx.S.chats)) {
    const found = list.find((one) => one.id === chatId);
    if (found) return found;
  }
  return undefined;
}

/**
 * The chat that is open in the pane. It is the one the project on screen has chosen, and only while
 * the Chats view is what is showing: a chat the person left open behind the board is not being read.
 */
function openChatOf(ctx: Ctx): Chat | undefined {
  const { route, chatOpen, chats } = ctx.S;
  if (route.page !== "project" || route.view !== "chat" || !route.pid) return undefined;
  const id = chatOpen[route.pid];
  return id ? chats[route.pid]?.find((one) => one.id === id) : undefined;
}

/** A chat whose history is being read: the events that arrive meanwhile wait, and a send waits too. */
interface Read {
  /** The events of the chat's topic that arrived before the history did, in order. */
  waiting: WireEvent[];
  /** Settles when the history is in, or could not be read. */
  done: Promise<void>;
}

/** The reads under way, by store: a store is one screen, and a test builds many. */
const READS = new WeakMap<Ctx, Map<string, Read>>();

function readsOf(ctx: Ctx): Map<string, Read> {
  const found = READS.get(ctx);
  if (found) return found;
  const made = new Map<string, Read>();
  READS.set(ctx, made);
  return made;
}

/**
 * Reads the open chat's history from the daemon and puts it in the chat, oldest first, in the shape
 * a card's chat has. The chat starts empty and says it is loading, so a chat that was open before
 * never shows an older read beside a newer one. What the chat's topic said while the page was on its
 * way is drawn after it, in order, so a turn that was running when the chat opened is not lost and a
 * message that is in both is not drawn twice. A read that fails says so in the chat, with the
 * daemon's own sentence, and can be tried again.
 *
 * A read that a newer read of the same chat overtakes (the chat was closed and opened again, or the
 * person pressed Try again) is dropped, so an older answer never lands on top of a newer one.
 */
export async function readOpenChat(ctx: Ctx, api: ApiClient, chat: Chat): Promise<void> {
  const chatId = chat.id;
  const reads = readsOf(ctx);
  let finish: () => void = () => undefined;
  const read: Read = { waiting: [], done: new Promise<void>((resolve) => (finish = resolve)) };
  reads.set(chatId, read);
  batch(() => {
    chat.msgs = [];
    chat.history = "loading";
    delete chat.historyError;
  });
  try {
    const page = await api.chatMessages(chatId, { limit: FIRST_PAGE });
    if (reads.get(chatId) !== read) return;
    batch(() => {
      chat.msgs = toStoredMessages(page.items);
      delete chat.history;
    });
  } catch (error) {
    if (reads.get(chatId) !== read) return;
    batch(() => {
      chat.history = "failed";
      chat.historyError = sentence(error, READ_FAILED);
    });
  } finally {
    if (reads.get(chatId) === read) {
      reads.delete(chatId);
      batch(() => {
        for (const event of read.waiting) applyToChat(ctx, chat, event);
      });
    }
    finish();
  }
}

/**
 * Keeps the event stream pointed at the chat that is open: it subscribes to that chat's own topic
 * when a chat opens, unsubscribes when it closes or another chat opens, and reads the chat's history
 * once, at the moment it opens. The chat's events (its words and its tool calls) arrive on
 * `chat:<id>`, so this belongs to the Chats view and not to the section-wide syncer, which follows
 * every project. It returns what re-reads the open chat, which the caller runs after a reconnection:
 * the stream does not replay what was said while it was away.
 */
export function followOpenChat(ctx: Ctx, api: ApiClient, stream: Stream): () => void {
  if (!isDaemon("S17", sectionsOf(ctx.env))) return () => undefined;
  let following: string | null = null;
  createEffect(() => {
    const chat = openChatOf(ctx);
    const wanted = chat?.id ?? null;
    if (wanted === following) return;
    if (following) stream.unsubscribe([chatTopic(following)]);
    following = wanted;
    if (!chat || !wanted) return;
    stream.subscribe([chatTopic(wanted)]);
    // The read writes to the chat and reads it, and the effect must not depend on either.
    untrack(() => void readOpenChat(ctx, api, chat));
  });
  return () => {
    const chat = untrack(() => openChatOf(ctx));
    if (chat) void readOpenChat(ctx, api, chat);
  };
}

/** The part of the event stream this needs: subscribing and unsubscribing to a topic. */
interface Stream {
  subscribe(topics: readonly string[]): void;
  unsubscribe(topics: readonly string[]): void;
}

/** Reads the chat again: the Try again of a chat whose history could not be read. */
export function retryChat(ctx: Ctx, pid: string, chatId: string): void {
  const api = ctx.env.data?.api;
  const chat = ctx.S.chats[pid]?.find((one) => one.id === chatId);
  if (!chat) return;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return;
  }
  void readOpenChat(ctx, api, chat);
}

/**
 * Applies one event of a chat's own topic to its messages, so an agent's words and tool calls appear
 * as they happen. A `session.output` chunk joins the agent message that is still streaming, or starts
 * one; a tool call is one line that each update finds by its id; and the session leaving its turn
 * ends the words that were arriving. A session that stopped says why, in the thread, in the
 * daemon's own sentence, because nothing else tells a person that a chat's agent has gone.
 *
 * While the chat's history is being read the event waits its turn. An event about a chat the store
 * does not hold, or one that names no chat (a card's), is left alone.
 */
export function applyChatSessionEvent(ctx: Ctx, event: WireEvent): void {
  if (
    event.type !== EventTypeSessionStateChanged &&
    event.type !== EventTypeSessionOutput &&
    event.type !== EventTypeSessionToolCall
  ) {
    return;
  }
  const data = isRecord(event.data) ? event.data : null;
  const chatId = data && typeof data.chatId === "string" ? data.chatId : "";
  const chat = chatId ? chatOf(ctx, chatId) : undefined;
  if (!chat) return;
  const waiting = readsOf(ctx).get(chatId);
  if (waiting) waiting.waiting.push(event);
  else applyToChat(ctx, chat, event);
}

function applyToChat(ctx: Ctx, chat: Chat, event: WireEvent): void {
  const data = isRecord(event.data) ? event.data : null;
  if (event.type === EventTypeSessionOutput) {
    applyOutput(chat.msgs, ctx, data);
    return;
  }
  if (event.type === EventTypeSessionToolCall && data) {
    applyToolCall(chat.msgs, ctx, data);
    return;
  }
  if (event.type !== EventTypeSessionStateChanged || !data) return;
  if (data.state !== SessionStateWorking) endStreaming(chat.msgs);
  const reason = typeof data.reason === "string" ? data.reason : "";
  if (data.state === SessionStateStopped && reason) {
    chat.msgs.push({ id: `s${takeMid(ctx.ids)}`, k: "system", text: reason });
  }
}

/** Takes one message out of a chat, by id: the array is the store's, and it hands back its own wrappers. */
function withdraw(chat: Chat, message: Msg): void {
  const at = chat.msgs.findIndex((one) => one.id === message.id);
  if (at >= 0) chat.msgs.splice(at, 1);
}

/**
 * Sends the person's own message into the chat's session, and shows it at once: the daemon does not
 * publish the message back as an event, so nothing else would draw it until the chat was reopened.
 * The daemon may refuse it (the chat is archived, its conversation cannot be picked back up, its
 * queue is full, its agent will not start), and then the words are taken back out and its own
 * sentence is shown, so the screen never keeps words that were never sent. A send waits for the
 * history that is being read, so the message never lands under an older read of the chat.
 *
 * The first message of a chat starts its agent, which can take a while, and the daemon names a chat
 * that is still "New chat" after it (`chat.updated`, which `chats.ts` draws).
 */
export async function sendToChat(
  ctx: Ctx,
  pid: string,
  chatId: string,
  text: string,
): Promise<boolean> {
  const chat = ctx.S.chats[pid]?.find((one) => one.id === chatId);
  const said = text.trim();
  if (!chat || !said) return false;
  const api = ctx.env.data?.api;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return false;
  }
  await readsOf(ctx).get(chatId)?.done;
  const shown: Msg = { id: `s${takeMid(ctx.ids)}`, k: "user", text: said };
  chat.msgs.push(shown);
  try {
    await api.sendChatMessage(chatId, { text: said });
    return true;
  } catch (error) {
    withdraw(chat, shown);
    toast(ctx, sentence(error, SEND_FAILED));
    return false;
  }
}
