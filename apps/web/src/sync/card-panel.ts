import type {
  CardCheck,
  Attachment as WireAttachment,
  Checklist as WireChecklist,
  Comment as WireComment,
  Event as WireEvent,
} from "@marshal/protocol";
import {
  EventTypeCardMembersChanged,
  EventTypeChecklistUpdated,
  EventTypeCommentCreated,
  EventTypeCommentReadByAgent,
} from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import { toMillis } from "~/data/mappers/time";
import { isDaemon } from "~/data/sections";
import type { CardKey } from "~/mock/card-key";
import { type Ctx, sectionsOf } from "~/mock/context";
import type { Attachment, Check, Checklist, CheckState, Comment } from "~/mock/types";

/*
 * What a card's panel holds beyond the card, from the daemon to the screens: acceptance checks
 * (section S12), checklists (S15), and comments and members (S16). A card opens with all of them
 * read once, and each section is read again when an event of the card's own topic says it changed,
 * so two browsers on one card agree. The daemon answers each call with the whole list, which is
 * what is put in the store, so there is no merging to get wrong.
 */

const MEGABYTE = 1e6;
const KILOBYTE = 1e3;

const sizeLabel = (bytes: number): string =>
  bytes > MEGABYTE
    ? `${(bytes / MEGABYTE).toFixed(1)} MB`
    : `${Math.max(1, Math.round(bytes / KILOBYTE))} KB`;

const checkState = (check: CardCheck): CheckState => check.status;

export const toCheck = (check: CardCheck): Check => ({
  id: check.id,
  name: check.name,
  cmd: check.command,
  st: checkState(check),
});

export function toChecklist(list: WireChecklist): Checklist {
  return {
    id: list.id,
    title: list.name,
    hideDone: list.hideChecked,
    required: list.required,
    peopleOnly: list.peopleOnly,
    items: list.items.map((item) => ({
      id: item.id,
      text: item.text,
      done: item.done,
      by: !item.done ? null : item.doneByKind === "agent" ? "agent" : item.doneById || null,
      ...(item.doneAt ? { doneAt: toMillis(item.doneAt) } : {}),
    })),
  };
}

function toAttachment(cardId: string, one: WireAttachment): Attachment {
  if (one.kind === "link") return { kind: "link", name: one.name, url: one.url };
  return {
    kind: one.kind,
    name: one.name,
    size: sizeLabel(one.sizeBytes),
    ref: { cardId, id: one.id },
  };
}

export const toComment = (cardId: string, comment: WireComment): Comment => ({
  id: comment.id,
  author: comment.authorKind === "agent" ? "agent" : comment.authorId,
  text: comment.body,
  ts: toMillis(comment.createdAt),
  att: comment.attachments.map((one) => toAttachment(cardId, one)),
  read: comment.agentReadAt !== null,
});

const cardOf = (ctx: Ctx, key: CardKey) => ctx.S.cards.find((one) => one.id === key);

export async function readChecks(ctx: Ctx, api: ApiClient, daemonId: string, key: CardKey) {
  const answer = await api.cardChecks(daemonId);
  if (cardOf(ctx, key)) ctx.S.checks[key] = answer.checks.map(toCheck);
}

async function readLists(ctx: Ctx, api: ApiClient, daemonId: string, key: CardKey) {
  const answer = await api.checklists(daemonId);
  const card = cardOf(ctx, key);
  if (card) card.checklists = answer.checklists.map(toChecklist);
}

/** Loads image bytes so a picture can be drawn: a file needs the token, so it cannot be a plain address. */
async function loadImages(ctx: Ctx, api: ApiClient, key: CardKey): Promise<void> {
  const card = cardOf(ctx, key);
  if (!card) return;
  for (const comment of card.comments) {
    for (const one of comment.att) {
      if (one.kind !== "image" || one.src || !one.ref) continue;
      try {
        const blob = await api.attachmentFile(one.ref.cardId, one.ref.id);
        one.src = URL.createObjectURL(blob);
      } catch {
        // The picture stays a name with nothing to show; opening it tries again.
      }
    }
  }
}

async function readComments(ctx: Ctx, api: ApiClient, daemonId: string, key: CardKey) {
  const answer = await api.comments(daemonId);
  const card = cardOf(ctx, key);
  if (!card) return;
  // A picture already loaded is kept, so a new comment does not make every picture flicker.
  const have = new Map<string, string>();
  for (const comment of card.comments) {
    for (const one of comment.att) if (one.ref && one.src) have.set(one.ref.id, one.src);
  }
  const next = answer.comments.map((one) => toComment(daemonId, one));
  for (const comment of next) {
    for (const one of comment.att) {
      const src = one.ref ? have.get(one.ref.id) : undefined;
      if (src) one.src = src;
    }
  }
  card.comments = next;
  void loadImages(ctx, api, key);
}

async function readMembers(ctx: Ctx, api: ApiClient, daemonId: string, key: CardKey) {
  const answer = await api.cardMembers(daemonId);
  const card = cardOf(ctx, key);
  if (card) card.members = answer.userIds;
}

/** Reads everything a card's panel holds that is the daemon's, once, when the card opens. */
export async function readPanel(
  ctx: Ctx,
  api: ApiClient,
  daemonId: string,
  key: CardKey,
): Promise<void> {
  const table = sectionsOf(ctx.env);
  const reads: Promise<unknown>[] = [];
  if (isDaemon("S12", table)) reads.push(readChecks(ctx, api, daemonId, key));
  if (isDaemon("S15", table)) reads.push(readLists(ctx, api, daemonId, key));
  if (isDaemon("S16", table)) {
    reads.push(readComments(ctx, api, daemonId, key), readMembers(ctx, api, daemonId, key));
  }
  // One section failing to read leaves that section as it was and lets the others draw.
  await Promise.allSettled(reads);
}

/** The panel sections this build of the store keeps on the daemon. */
export const panelOnDaemon = (ctx: Ctx): boolean => {
  const table = sectionsOf(ctx.env);
  return isDaemon("S12", table) || isDaemon("S15", table) || isDaemon("S16", table);
};

/**
 * One event of a card's own topic that says a part of its panel changed: that part is read again.
 * The event names only the card, so the whole list is read and put in place.
 */
export function applyPanelEvent(ctx: Ctx, event: WireEvent): void {
  const data = isRecord(event.data) ? event.data : null;
  const daemonId = data && typeof data.cardId === "string" ? data.cardId : "";
  const api = ctx.env.data?.api;
  const card = daemonId ? ctx.S.cards.find((one) => one.daemonId === daemonId) : undefined;
  if (!api || !card) return;
  const table = sectionsOf(ctx.env);
  const key = card.id;
  let read: Promise<unknown> | null = null;
  switch (event.type) {
    case EventTypeChecklistUpdated:
      if (isDaemon("S15", table)) read = readLists(ctx, api, daemonId, key);
      if (isDaemon("S12", table)) read = Promise.all([read, readChecks(ctx, api, daemonId, key)]);
      break;
    case EventTypeCommentCreated:
    case EventTypeCommentReadByAgent:
      if (isDaemon("S16", table)) read = readComments(ctx, api, daemonId, key);
      break;
    case EventTypeCardMembersChanged:
      if (isDaemon("S16", table)) read = readMembers(ctx, api, daemonId, key);
      break;
    default:
      return;
  }
  // A read that fails leaves the list as it was; the next event or the next open reads it again.
  if (read) void read.catch(() => undefined);
}
