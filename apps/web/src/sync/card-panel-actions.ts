import type { NewAttachment } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { ApiError } from "~/data/api-error";
import type { CardKey } from "~/mock/card-key";
import type { Ctx } from "~/mock/context";
import { confirm, toast } from "~/mock/engine";
import type { Attachment, Card } from "~/mock/types";
import { toCheck, toChecklist, toComment } from "./card-panel";

/*
 * The writes of a card's panel once its sections are the daemon's: checklists (S15), comments and
 * members (S16), and the acceptance checks (S12). Each one keeps the arguments of the mock's own
 * function it stands in for. The daemon answers every call with the whole list, and that answer is
 * what the store takes, so the screen never draws what the daemon did not agree to. A refusal is shown
 * as the daemon's own sentence.
 */

const NOT_CONNECTED = "Marshal is not connected to its daemon.";
const GENERIC_FAILURE = "Marshal could not finish that. Try again.";

interface Target {
  api: ApiClient;
  card: Card;
  daemonId: string;
}

function targetOf(ctx: Ctx, key: CardKey): Target | null {
  const card = ctx.S.cards.find((one) => one.id === key);
  if (!card?.daemonId) return null;
  const api = ctx.env.data?.api;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return null;
  }
  return { api, card, daemonId: card.daemonId };
}

/** Runs one panel call. A failure is a toast with the daemon's sentence, and the store is left alone. */
async function run<T>(
  ctx: Ctx,
  key: CardKey,
  call: (t: Target) => Promise<T>,
  put: (t: Target, answer: T) => void,
) {
  const target = targetOf(ctx, key);
  if (!target) return;
  try {
    put(target, await call(target));
  } catch (error) {
    toast(ctx, error instanceof ApiError ? error.message : GENERIC_FAILURE);
  }
}

type ListAnswer = Awaited<ReturnType<ApiClient["checklists"]>>;
const putLists = (t: Target, answer: ListAnswer): void => {
  t.card.checklists = answer.checklists.map(toChecklist);
};

export function toggleItem(ctx: Ctx, cid: CardKey, lid: string, iid: string): void {
  const item = ctx.S.cards
    .find((c) => c.id === cid)
    ?.checklists.find((l) => l.id === lid)
    ?.items.find((i) => i.id === iid);
  if (!item) return;
  void run(
    ctx,
    cid,
    (t) => t.api.tickChecklistItem(t.daemonId, lid, iid, { done: !item.done }),
    putLists,
  );
}

export function addItem(ctx: Ctx, cid: CardKey, lid: string, text: string): void {
  if (!text?.trim()) return;
  void run(
    ctx,
    cid,
    (t) => t.api.addChecklistItem(t.daemonId, lid, { text: text.trim() }),
    putLists,
  );
}

export function removeItem(ctx: Ctx, cid: CardKey, lid: string, iid: string): void {
  void run(ctx, cid, (t) => t.api.removeChecklistItem(t.daemonId, lid, iid), putLists);
}

export function addChecklist(ctx: Ctx, cid: CardKey, title?: string): void {
  void run(
    ctx,
    cid,
    (t) => t.api.createChecklist(t.daemonId, { name: (title ?? "").trim() }),
    (t, answer) => {
      putLists(t, answer);
      toast(ctx, "Checklist added");
    },
  );
}

export function deleteChecklist(ctx: Ctx, cid: CardKey, lid: string): void {
  const list = ctx.S.cards.find((c) => c.id === cid)?.checklists.find((l) => l.id === lid);
  if (!list) return;
  confirm(ctx, {
    title: "Delete checklist",
    message: `This deletes "${list.title}" and its ${list.items.length} items.`,
    action: "Delete checklist",
    destructive: true,
    run: () =>
      void run(
        ctx,
        cid,
        (t) => t.api.deleteChecklist(t.daemonId, lid),
        (t, answer) => {
          putLists(t, answer);
          toast(ctx, "Checklist deleted");
        },
      ),
  });
}

export function toggleHideDone(ctx: Ctx, cid: CardKey, lid: string): void {
  const list = ctx.S.cards.find((c) => c.id === cid)?.checklists.find((l) => l.id === lid);
  if (!list) return;
  void run(
    ctx,
    cid,
    (t) => t.api.updateChecklist(t.daemonId, lid, { hideChecked: !list.hideDone }),
    putLists,
  );
}

/** The bytes of a picked file, from the address the composer made for it, as base64. */
async function base64Of(blob: Blob): Promise<string> {
  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result).split(",")[1] ?? "");
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
}

async function newAttachments(list: readonly Attachment[]): Promise<NewAttachment[]> {
  const out: NewAttachment[] = [];
  for (const one of list) {
    if (one.kind === "link") {
      out.push({ kind: "link", name: one.name, mimeType: "", url: one.url ?? "", data: "" });
      continue;
    }
    if (!one.src) continue;
    const blob = await (await fetch(one.src)).blob();
    out.push({
      kind: one.kind,
      name: one.name,
      mimeType: blob.type,
      url: "",
      data: await base64Of(blob),
    });
  }
  return out;
}

export function addComment(ctx: Ctx, cid: CardKey, text: string, att?: Attachment[]): void {
  if (!text.trim() && !att?.length) return;
  const target = targetOf(ctx, cid);
  if (!target) return;
  void newAttachments(att ?? [])
    .then((attachments) => target.api.postComment(target.daemonId, { body: text, attachments }))
    .then((answer) => {
      target.card.comments = answer.comments.map((one) => toComment(target.daemonId, one));
      toast(ctx, "Comment added");
    })
    .catch((error: unknown) =>
      toast(ctx, error instanceof ApiError ? error.message : GENERIC_FAILURE),
    );
}

export function deleteComment(ctx: Ctx, cid: CardKey, id: string): void {
  void run(
    ctx,
    cid,
    (t) => t.api.deleteComment(t.daemonId, id),
    (t, answer) => {
      t.card.comments = answer.comments.map((one) => toComment(t.daemonId, one));
      toast(ctx, "Comment deleted");
    },
  );
}

export function toggleMember(ctx: Ctx, cid: CardKey, pid: string): void {
  const on = ctx.S.cards.find((c) => c.id === cid)?.members.includes(pid);
  void run(
    ctx,
    cid,
    (t) => (on ? t.api.removeCardMember(t.daemonId, pid) : t.api.addCardMember(t.daemonId, pid)),
    (t, answer) => {
      t.card.members = answer.userIds;
    },
  );
}

/** Runs the card's command checks. Each shows as running until the daemon has the answer. */
export function runChecks(ctx: Ctx, cid: CardKey): void {
  const checks = ctx.S.checks[cid];
  if (checks) for (const check of checks) if (check.cmd) check.st = "running";
  const target = targetOf(ctx, cid);
  if (!target) return;
  void target.api
    .runCardChecks(target.daemonId)
    .then((answer) => {
      ctx.S.checks[cid] = answer.checks.map(toCheck);
    })
    .catch(async (error: unknown) => {
      // A refused run leaves the checks as they were, so they are read again rather than left running.
      toast(ctx, error instanceof ApiError ? error.message : GENERIC_FAILURE);
      try {
        ctx.S.checks[cid] = (await target.api.cardChecks(target.daemonId)).checks.map(toCheck);
      } catch {
        // The list stays as it is until the card is opened again.
      }
    });
}

/** Opens a file kept with a comment: it is fetched with the token and handed to the browser. */
export async function openAttachment(ctx: Ctx, att: Attachment): Promise<void> {
  const api = ctx.env.data?.api;
  if (!api || !att.ref) return;
  try {
    const blob = await api.attachmentFile(att.ref.cardId, att.ref.id);
    const link = document.createElement("a");
    link.href = URL.createObjectURL(blob);
    link.download = att.name;
    link.click();
    setTimeout(() => URL.revokeObjectURL(link.href), 0);
  } catch (error) {
    toast(ctx, error instanceof ApiError ? error.message : GENERIC_FAILURE);
  }
}
