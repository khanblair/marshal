/**
 * The project chat routes of the fake daemon (docs/backend-checklist.md B2.10), so the unit tests
 * that follow the cutover can drive the real API client and the real chats syncer without a
 * network. Each route answers the way the daemon's own handler does, including the `chat.*` event
 * it publishes after a change: a test that archives a chat sees the event arrive on the project's
 * topic.
 *
 * Only the fields the daemon fills are made here. The shapes come from the golden files, which the
 * Go tests wrote from the real wire types.
 */
import type {
  ChatMessage,
  ChatMessageDetail,
  CreateChatRequest,
  SendMessageRequest,
  UpdateChatRequest,
  Chat as WireChat,
  ChatTarget as WireChatTarget,
} from "@marshal/protocol";
import { emptyAnswer, errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";
import { DEFAULT_LIMIT, pageOf, queryOf } from "./fake-card-shared";

const STATUS = { created: 201, badRequest: 400, notFound: 404, refused: 422 };

/** One stored message of a chat: the chat it belongs to, and what the daemon recorded for it. */
export interface ChatMessageRow {
  chatId: string;
  message: ChatMessage;
}

/** A stored chat message a test sets up: the chat it belongs to, and what the daemon recorded. */
export function chatMessageRow(
  chatId: string,
  message: Partial<ChatMessage> & Pick<ChatMessage, "id" | "kind">,
): ChatMessageRow {
  return {
    chatId,
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
  };
}

/** A chat the fake daemon holds: the wire type, and the project whose routes list it. */
export function wireChat(fields: Partial<WireChat> & { id: string; projectId: string }): WireChat {
  const base = structuredClone(golden<WireChat>("chat"));
  return { ...base, ...fields, id: fields.id, projectId: fields.projectId };
}

/** Where the ids of chats the fake daemon makes start, so they cannot collide with a fixture's. */
const MADE_CHAT_BASE = 800_000;

/** How many digits of the id a made chat's counter fills. The id itself is a 26-character ULID. */
const ID_DIGITS = 16;

/** How many ids the fake daemon has handed out: one per chat it made, so no two share one. */
let madeChats = 0;

/**
 * A fresh opaque id, unique among the chats this fake daemon made. It has the shape of the
 * daemon's own ids (26 Crockford base32 characters starting with a digit below 8), so a test that
 * checks an id's shape sees what the real daemon would have sent.
 */
function freshChatId(): string {
  madeChats += 1;
  return `01M3CHATS${String(MADE_CHAT_BASE + madeChats).padStart(ID_DIGITS, "0")}`;
}

/** The chats the fake daemon holds, and the two things it needs to answer a change. */
export interface ChatStore {
  /** Every chat the daemon holds, across projects. */
  chats: WireChat[];
  /** The stored messages of every chat, as the daemon's own history keeps them (any order). */
  messages: ChatMessageRow[];
  /**
   * What a chat's agent answers to a message. The real one runs a program; this fake says one plain
   * sentence, so a test can see a scripted answer come in on the chat's topic.
   */
  answer?: (text: string) => string;
  /** Sends an event on a topic, as the daemon does after a change. */
  publish: (topic: string, type: string, data: unknown) => void;
  now: () => string;
}

const topic = (projectId: string): string => `project:${projectId}`;

const notFound = () =>
  errorAnswer(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that chat. It may have been removed.",
  );

const noProject = () =>
  errorAnswer(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that project. It may have been removed.",
  );

/** Reads the JSON body of a request. A body that is not JSON is `{}`. */
function bodyOf(request: FakeRequest): Record<string, unknown> {
  try {
    return request.body ? (JSON.parse(request.body) as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

/** The target a create asked for, with the Orchestrator as the default the daemon applies. */
function targetOf(value: unknown): WireChatTarget {
  if (!value || typeof value !== "object") return { kind: "orchestrator", id: "" };
  const target = value as Partial<WireChatTarget>;
  const kind = target.kind ?? "orchestrator";
  return { kind, id: typeof target.id === "string" ? target.id : "" };
}
/** Makes a chat in a project and announces it. */
function createChat(
  { chats, publish, now }: ChatStore,
  projectId: string,
  request: FakeRequest,
): Response {
  const body = bodyOf(request) as CreateChatRequest;
  const title = typeof body.title === "string" ? body.title.trim() : "";
  if (body.title !== undefined && !title) {
    return errorAnswer(
      STATUS.badRequest,
      "invalid_argument",
      "Chat names can't be empty. The old name is kept.",
    );
  }
  const chat = wireChat({
    id: freshChatId(),
    projectId,
    title: title || "New chat",
    target: targetOf(body.target),
    archivedAt: null,
    lastActiveAt: now(),
    createdAt: now(),
  });
  chats.push(chat);
  publish(topic(projectId), "chat.created", { chat });
  return jsonAnswer(chat, STATUS.created);
}

/** Renames a chat and announces it. */
function updateChat({ publish }: ChatStore, chat: WireChat, request: FakeRequest): Response {
  const body = bodyOf(request) as UpdateChatRequest;
  if (body.title !== undefined) {
    const title = typeof body.title === "string" ? body.title.trim() : "";
    if (!title) {
      return errorAnswer(
        STATUS.badRequest,
        "invalid_argument",
        "Chat names can't be empty. The old name is kept.",
      );
    }
    chat.title = title;
  }
  publish(topic(chat.projectId), "chat.updated", { chat });
  return jsonAnswer(chat);
}

/** Archives or restores a chat and announces it. One event type serves both directions. */
function setArchived({ publish, now }: ChatStore, chat: WireChat, archived: boolean): Response {
  chat.archivedAt = archived ? now() : null;
  publish(topic(chat.projectId), "chat.archived", { chat });
  return jsonAnswer(chat);
}

/** Deletes a chat and announces it. */
function removeChat({ chats, messages, publish }: ChatStore, chat: WireChat): Response {
  chats.splice(chats.indexOf(chat), 1);
  messages.splice(0, messages.length, ...messages.filter((row) => row.chatId !== chat.id));
  publish(topic(chat.projectId), "chat.deleted", { chatId: chat.id, projectId: chat.projectId });
  return emptyAnswer();
}

/** How many words of a first message name the chat, and where the name stops (`chats.titleOf`). */
const TITLE_WORDS = 6;

/** The name the daemon gives a chat from its first message: the first words, without a closing mark. */
function titleFrom(text: string): string {
  const words = text
    .replace(/[?.!]+$/, "")
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, TITLE_WORDS)
    .join(" ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** A chat's stored messages, newest first, which is the order a page is read in. */
const historyOf = (store: ChatStore, chatId: string): ChatMessage[] =>
  store.messages
    .filter((row) => row.chatId === chatId)
    .map((row) => row.message)
    .sort((a, b) => b.seq - a.seq);

/** Records one message in a chat's history, numbered after the newest. */
function record(
  store: ChatStore,
  chat: WireChat,
  message: Partial<ChatMessage> & Pick<ChatMessage, "id" | "kind">,
): void {
  const seq = Math.max(0, ...historyOf(store, chat.id).map((one) => one.seq)) + 1;
  store.messages.push(chatMessageRow(chat.id, { ...message, seq, at: store.now() }));
}

/** The events of a chat's own topic, as the daemon publishes them (`session.*` with `chatId`). */
function publishSession(store: ChatStore, chat: WireChat, type: string, data: object): void {
  store.publish(`chat:${chat.id}`, type, { cardId: "", chatId: chat.id, ...data });
}

/** How many digits of the id a recorded message's counter fills. */
const MESSAGE_ID_DIGITS = 15;

/** A message id that is unique among what this fake recorded. */
let recorded = 0;
const messageId = (): string => `01M3CHATMSG${String(++recorded).padStart(MESSAGE_ID_DIGITS, "0")}`;

/**
 * POST /v1/chats/{id}/messages, as the daemon answers it: an empty message is refused, so is one to an
 * archived chat (with the stable reason a client branches on), and any other is accepted with no body.
 * The message is recorded, the chat is touched and, while it is still "New chat", named after the
 * message (`chat.updated`), and then the agent works: its session goes working, says its answer, and
 * goes back to waiting, each on the chat's own topic and after the answer to the request, as a real
 * agent's would be.
 */
function sendChatMessage(store: ChatStore, chat: WireChat, request: FakeRequest): Response {
  const { text = "" } = bodyOf(request) as Partial<SendMessageRequest>;
  if (typeof text !== "string" || !text.trim()) {
    return errorAnswer(STATUS.badRequest, "invalid_argument", "Write a message first.");
  }
  if (chat.archivedAt !== null) {
    return jsonAnswer(
      {
        error: {
          code: "refused",
          message: "This chat is archived. Restore it to keep talking.",
          details: { reason: "chat_archived", chatId: chat.id },
        },
      },
      STATUS.refused,
    );
  }
  record(store, chat, { id: messageId(), kind: "user", text });
  if (chat.title === "New chat") chat.title = titleFrom(text);
  chat.lastActiveAt = store.now();
  store.publish(topic(chat.projectId), "chat.updated", { chat });
  const reply = store.answer?.(text) ?? `The daemon answered: ${text}`;
  queueMicrotask(() => {
    publishSession(store, chat, "session.state_changed", {
      sessionId: "01M3CHATSESSION",
      state: "working",
    });
    publishSession(store, chat, "session.output", { kind: "message", text: reply });
    record(store, chat, { id: messageId(), kind: "agent", text: reply });
    publishSession(store, chat, "session.state_changed", {
      sessionId: "01M3CHATSESSION",
      state: "awake",
    });
  });
  return emptyAnswer();
}

/** GET /v1/chats/{id}/messages: the stored history, newest first, one page at a time. */
function chatMessages(store: ChatStore, chat: WireChat, request: FakeRequest): Response {
  const query = queryOf(request.url);
  const limit = Number(query.get("limit")) || DEFAULT_LIMIT;
  return jsonAnswer(
    pageOf(historyOf(store, chat.id), limit, query.get("cursor") ?? "", store.now()),
  );
}

/** GET /v1/chats/{id}/messages/{messageId}: one message in full. */
function chatMessage(store: ChatStore, chat: WireChat, messageId: string): Response {
  const message = historyOf(store, chat.id).find((one) => one.id === messageId);
  if (!message) return notFound();
  const detail: ChatMessageDetail = { message, tool: null, serverTime: store.now() };
  return jsonAnswer(detail);
}

/** A route under a project's chats: list them, or make one. */
function projectChatsRoute(
  store: ChatStore,
  request: FakeRequest,
  projectId: string,
  exists: boolean,
): Response | undefined {
  if (!exists) {
    return request.method === "POST" || request.method === "GET" ? noProject() : undefined;
  }
  if (request.method === "POST") return createChat(store, projectId, request);
  if (request.method !== "GET") return undefined;
  const archived = request.url.includes("archived=true");
  const chats = store.chats.filter(
    (chat) => chat.projectId === projectId && (chat.archivedAt !== null) === archived,
  );
  return jsonAnswer({ projectId, chats, serverTime: store.now() });
}

/** A route under one chat: rename, delete, archive, restore, or its messages. */
function chatRoute(
  store: ChatStore,
  request: FakeRequest,
  chat: WireChat,
  action: string | undefined,
  extra: string | undefined,
): Response | undefined {
  if (!action && request.method === "PATCH") return updateChat(store, chat, request);
  if (!action && request.method === "DELETE") return removeChat(store, chat);
  if (action === "messages") {
    if (request.method === "POST" && !extra) return sendChatMessage(store, chat, request);
    if (request.method !== "GET") return undefined;
    return extra ? chatMessage(store, chat, extra) : chatMessages(store, chat, request);
  }
  if (action === "archive" && request.method === "POST") return setArchived(store, chat, true);
  if (action === "restore" && request.method === "POST") return setArchived(store, chat, false);
  return undefined;
}

/**
 * One request under a project's chats or under one chat. It returns undefined for a path that
 * belongs to neither, so the caller can answer 404 the way the real daemon's router does.
 */
export function answerChatRoute(
  store: ChatStore,
  request: FakeRequest,
  projectExists: (id: string) => boolean,
): Response | undefined {
  const path = request.url.replace(/^https?:\/\/[^/]+/, "").split("?")[0] ?? "";
  // `["v1", <kind>, <id>, <action>, <extra>]`, read back the way the real router's path values are.
  const [, kind, first, second, third] = path
    .split("/")
    .filter(Boolean)
    .map((segment) => decodeURIComponent(segment));
  if (kind === "projects" && first && second === "chats") {
    return projectChatsRoute(store, request, first, projectExists(first));
  }
  if (kind !== "chats" || !first) return undefined;
  const chat = store.chats.find((one) => one.id === first);
  return chat ? chatRoute(store, request, chat, second, third) : notFound();
}
