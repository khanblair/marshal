import {
  EventTypeChatArchived,
  EventTypeChatCreated,
  EventTypeChatDeleted,
  EventTypeChatUpdated,
  type Chat as WireChat,
  type ChatTarget as WireChatTarget,
  type Event as WireEvent,
} from "@marshal/protocol";
import { batch } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import { toMillis } from "~/data/mappers/time";
import type { Ctx } from "~/mock/context";
import type { Card, Chat } from "~/mock/types";
import type { Syncer } from "./syncer";

/**
 * Section S17: a project's chats, and the `chat.*` events of every project's topic.
 *
 * The chats view reads `S.chats[pid]`, a `Chat` per list row, so this module is the one place that
 * knows both languages:
 *
 *   - The wire splits a chat's target into a kind and an id (the Orchestrator, a role name, or a
 *     card's opaque id). The store keeps one string, which is what the view shows and what a new
 *     chat picks: "Orchestrator", a role name, or a card key such as `web#118`.
 *   - The wire has timestamps in UTC; the list draws "4 min ago", so `last` is milliseconds.
 *   - The wire has `archivedAt` (null while the chat is in the main list); the store has
 *     `archived`.
 *
 * A chat's messages are not read here. They are read when a chat is opened and follow its own topic
 * (`chat-session.ts`), so this module never writes a chat's `msgs`, or the state of its history read:
 * those fields belong to that module. A chat that is already in the store keeps them.
 */
export const chatsSyncer: Syncer<WireChat[]> = {
  section: "S17",
  topics: [],
  projectTopics: (projectID) => [`project:${projectID}`],
  async load(api: ApiClient, ctx: Ctx) {
    // One pair of lists per project: there is no call that lists chats across projects, and the
    // view keeps the archived ones behind its Archived toggle, so both halves are read.
    const projects = ctx.S.projects.map((project) => project.id);
    const lists = await Promise.all(
      projects.map(async (projectId) => {
        const [live, archived] = await Promise.all([
          api.listChats(projectId),
          api.listChats(projectId, true),
        ]);
        return [...live.chats, ...archived.chats];
      }),
    );
    return lists.flat();
  },
  apply: applyChatSnapshot,
  onEvent: applyChatEvent,
};

/**
 * One chat, in the shape the store keeps. `msgs` is empty: a chat's messages are read when it is
 * opened (`chat-session.ts`), and a chat that is already in the store keeps the messages it has.
 */
export function storedChat(wire: WireChat, cards: readonly Card[] = []): Chat {
  return {
    id: wire.id,
    pid: wire.projectId,
    title: wire.title,
    target: targetOf(wire.target, cards),
    msgs: [],
    last: toMillis(wire.lastActiveAt),
    archived: wire.archivedAt !== null,
    fresh: false,
  };
}

/**
 * The one string the view shows for a target. The Orchestrator's target has no id, a role's id is
 * its name, and a card's id is opaque: the store knows a card by its key, so the key is taken from
 * the card the daemon sent on the project's board. A card that is not in the store yet (the board
 * has not been applied, or the card is gone) leaves the opaque id, which is what the daemon sent.
 */
export function targetOf(target: WireChatTarget, cards: readonly Card[] = []): string {
  switch (target.kind) {
    case "orchestrator":
      return "Orchestrator";
    case "role":
      return target.id;
    case "card":
      return cards.find((card) => card.daemonId === target.id)?.id ?? target.id;
    default:
      return target.id;
  }
}

/**
 * The wire target of a store target string, which is `targetOf` read backwards: the Orchestrator has
 * no id, a card key becomes the card's opaque id (the wire names a card by it, never by its key), and
 * every other string is a role's own name. A key that names no card in the store is taken for a role,
 * and the daemon says so if it is not one.
 */
export function wireTargetOf(target: string, cards: readonly Card[] = []): WireChatTarget {
  if (target === "Orchestrator") return { kind: "orchestrator", id: "" };
  const card = cards.find((one) => one.id === target);
  if (card?.daemonId) return { kind: "card", id: card.daemonId };
  return { kind: "role", id: target };
}

/**
 * Makes the store's chats the daemon's, project by project. A chat that is already there has the
 * daemon's own fields updated in place and keeps everything the app owns, above all its messages.
 * A chat the daemon no longer lists is forgotten. Applying the same lists twice changes nothing.
 */
export function applyChatSnapshot(ctx: Ctx, chats: readonly WireChat[]): void {
  const { S } = ctx;
  batch(() => {
    const byProject = new Map<string, WireChat[]>();
    for (const chat of chats) {
      const list = byProject.get(chat.projectId);
      if (list) list.push(chat);
      else byProject.set(chat.projectId, [chat]);
    }
    // Every project already in the store is reconciled too, not just the ones the new snapshot
    // mentions: a project the daemon now answers with zero chats never appears in `byProject`, and
    // its stale rows would otherwise never be cleared out.
    const projectIds = new Set([...Object.keys(S.chats), ...byProject.keys()]);
    for (const projectId of projectIds) {
      const wires = byProject.get(projectId) ?? [];
      const current = S.chats[projectId] ?? [];
      const byId = new Map(current.map((chat) => [chat.id, chat]));
      const wanted = new Set(wires.map((chat) => chat.id));
      for (const wire of wires) {
        const existing = byId.get(wire.id);
        if (existing) mirrorChat(existing, wire, S.cards);
        else S.chats[projectId] = [...(S.chats[projectId] ?? []), storedChat(wire, S.cards)];
      }
      const kept = (S.chats[projectId] ?? []).filter((chat) => wanted.has(chat.id));
      if (kept.length !== (S.chats[projectId] ?? []).length) S.chats[projectId] = kept;
    }
  });
}

/** One chat created or changed on the daemon. Applying the same one twice changes nothing. */
export function applyChat(ctx: Ctx, wire: WireChat): void {
  const { S } = ctx;
  batch(() => {
    const list = S.chats[wire.projectId] ?? [];
    S.chats[wire.projectId] = list;
    const existing = list.find((chat) => chat.id === wire.id);
    if (existing) mirrorChat(existing, wire, S.cards);
    else list.push(storedChat(wire, S.cards));
  });
}

/**
 * A chat removed on the daemon, by its id. A chat that is open in the pane is closed: the pane
 * would otherwise draw a chat that is gone.
 */
export function applyChatRemoved(ctx: Ctx, chatId: string, projectId: string): void {
  const { S } = ctx;
  batch(() => {
    const list = S.chats[projectId];
    if (list) S.chats[projectId] = list.filter((chat) => chat.id !== chatId);
    if (S.chatOpen[projectId] === chatId) S.chatOpen[projectId] = null;
  });
}

/**
 * Writes the daemon's values into a chat that is already in the store, one field at a time, so a
 * field that changed redraws and one that did not, does not. `msgs` and `fresh` are never touched:
 * they are the app's own.
 */
function mirrorChat(target: Chat, wire: WireChat, cards: readonly Card[]): void {
  const next = storedChat(wire, cards);
  if (target.title !== next.title) target.title = next.title;
  if (target.target !== next.target) target.target = next.target;
  if (target.last !== next.last) target.last = next.last;
  if (target.archived !== next.archived) target.archived = next.archived;
}

/** The `chat.*` events of a project's topic. Each carries the chat as it is now. */
function applyChatEvent(ctx: Ctx, event: WireEvent): void {
  if (event.type === EventTypeChatDeleted) {
    // The payload carries the chat's id and its project, which is what the store is keyed by.
    if (
      isRecord(event.data) &&
      typeof event.data.chatId === "string" &&
      typeof event.data.projectId === "string"
    ) {
      applyChatRemoved(ctx, event.data.chatId, event.data.projectId);
    }
    return;
  }
  if (
    event.type !== EventTypeChatCreated &&
    event.type !== EventTypeChatUpdated &&
    event.type !== EventTypeChatArchived
  ) {
    return;
  }
  if (isRecord(event.data) && isRecord(event.data.chat)) {
    applyChat(ctx, event.data.chat as unknown as WireChat);
  }
}
