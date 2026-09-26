import { batch } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { ChangeInFlightError } from "~/data/optimistic";
import type { Ctx } from "~/mock/context";
import { confirm, toast } from "~/mock/engine";
import type { Chat } from "~/mock/types";
import { applyChat, applyChatRemoved, wireTargetOf } from "./chats";

/*
 * The write path of the project chats (section S17, docs/backend-checklist.md B2.10): make a chat,
 * rename it, archive and restore it, and delete it. Each is the daemon-backed version of one the
 * Chats view calls on the mock today, and keeps that one's arguments. What a person sees is what the
 * daemon says: a success is put in the store through the same functions the `chat.*` events use
 * (`applyChat`, `applyChatRemoved`), so the event that follows finds the chat as it already is, and a
 * refusal is the daemon's own sentence as a toast with nothing changed. Nothing here decides whether
 * a name is allowed or a chat may go: that is the daemon's.
 *
 * Opening a chat, and which one is open, are the screen's own state and stay on the mock's `openChat`.
 */

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

/**
 * Asks the daemon for one change and hands its answer back, or null when nothing was asked or the
 * daemon refused it. `optimistic` shows a refusal as the daemon's own sentence, and refuses a second
 * change to the same chat while one is still being saved.
 */
async function ask<T>(
  ctx: Ctx,
  chatId: string,
  run: (api: ApiClient) => Promise<T>,
): Promise<T | null> {
  const api = ctx.env.data?.api;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return null;
  }
  try {
    return await ctx.optimistic({
      key: `chat:${chatId}`,
      apply: () => undefined,
      request: () => run(api),
      rollback: () => undefined,
    });
  } catch (error) {
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
    return null;
  }
}

/**
 * Makes a chat that talks to `target` (the Orchestrator, a role's name, or a card's key) and opens
 * it, as the mock's New chat did. The daemon names it "New chat" and its first message renames it.
 * Answers the chat as the store holds it, or null when the daemon refused.
 */
export async function newChat(ctx: Ctx, pid: string, target?: string): Promise<Chat | null> {
  const made = await ask(ctx, `new:${pid}`, (api) =>
    api.createChat(pid, { target: wireTargetOf(target ?? "Orchestrator", ctx.S.cards) }),
  );
  if (!made) return null;
  batch(() => {
    applyChat(ctx, made);
    ctx.S.chatOpen[made.projectId] = made.id;
  });
  return ctx.S.chats[made.projectId]?.find((chat) => chat.id === made.id) ?? null;
}

/** Renames a chat. The daemon refuses a name that is only spaces, and keeps the old one. */
export async function renameChat(
  ctx: Ctx,
  pid: string,
  id: string,
  title: string,
): Promise<boolean> {
  if (!ctx.S.chats[pid]?.some((chat) => chat.id === id)) return false;
  const renamed = await ask(ctx, id, (api) => api.updateChat(id, { title }));
  if (!renamed) return false;
  applyChat(ctx, renamed);
  return true;
}

/**
 * Archives a chat, which puts its session to sleep, or restores an archived one. An archived chat
 * that was open is closed, and an archive can be undone from its toast, as on the mock.
 */
export async function archiveChat(ctx: Ctx, pid: string, id: string, on: boolean): Promise<void> {
  if (!ctx.S.chats[pid]?.some((chat) => chat.id === id)) return;
  const changed = await ask(ctx, id, (api) => (on ? api.archiveChat(id) : api.restoreChat(id)));
  if (!changed) return;
  batch(() => {
    applyChat(ctx, changed);
    if (on && ctx.S.chatOpen[pid] === id) ctx.S.chatOpen[pid] = null;
  });
  if (!on) {
    toast(ctx, "Chat restored");
    return;
  }
  toast(ctx, "Chat archived", {
    label: "Undo",
    run: () => void archiveChat(ctx, pid, id, false),
  });
}

/** Asks first, then deletes the chat, its session, and its logs. The cards it created stay. */
export function deleteChat(ctx: Ctx, pid: string, id: string): void {
  const chat = ctx.S.chats[pid]?.find((one) => one.id === id);
  if (!chat) return;
  confirm(ctx, {
    title: "Delete chat",
    message: `This deletes "${chat.title}" and its messages. Cards it created stay on the board.`,
    action: "Delete chat",
    destructive: true,
    run: () => void removeChat(ctx, pid, id),
  });
}

async function removeChat(ctx: Ctx, pid: string, id: string): Promise<void> {
  if ((await ask(ctx, id, (api) => api.deleteChat(id))) === null) return;
  batch(() => {
    applyChatRemoved(ctx, id, pid);
    toast(ctx, "Chat deleted");
  });
}
