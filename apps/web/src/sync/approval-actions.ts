import type { Event as WireEvent } from "@marshal/protocol";
import { EventTypeApprovalRequested, EventTypeApprovalResolved } from "@marshal/protocol";
import { ApiError } from "~/data/api-error";
import { isRecord } from "~/data/guards";
import { isDaemon } from "~/data/sections";
import type { CardKey } from "~/mock/card-key";
import { type Ctx, sectionsOf } from "~/mock/context";
import { toast } from "~/mock/engine";
import { takeMid } from "~/mock/ids";
import { card as cardOf } from "~/mock/selectors";
import type { ApprovalMsg, ApprovalState } from "~/mock/types";
import { platform } from "~/platform";

/*
 * The approval flow's own writes and its two live events (section S8b, B3.4, N7): the answer a
 * person gives to a request an agent is blocked on, and the request and the answer as they arrive
 * on an open card's own topic.
 *
 * There is one row and many views (N7): the card's chat and Home's needs-you card both draw the
 * same approval, so answering it here is one route call - `POST /v1/approvals/{id}` takes the
 * approval's own id, never the card's - and every view follows the same approval.requested/
 * approval.resolved pair the daemon publishes rather than a write of its own. This mirrors how
 * `plan-actions.ts` cut plans over for section S8c, with one difference the daemon's own answer has
 * no body: the card the approval belonged to already moves with `card.moved`/`card.updated`
 * (`sync/cards.ts`'s generic handling of every card, section S5a), so nothing here has to apply a
 * card of its own.
 */

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

/** True while approvals are the daemon's (S8b). */
export function approvalsOnDaemon(ctx: Ctx): boolean {
  return isDaemon("S8b", sectionsOf(ctx.env));
}

/**
 * The id of the approval a card is waiting on right now, read the way that answers whether or not
 * the card is open. `Card.approvalId` is set from the daemon's own NeedsReason (S8b's Home gap) and
 * is read fresh on every card update, so it is the one that works for a card nobody has opened; the
 * open chat's own waiting approval block is read too, as a fallback for the narrow moment before a
 * `card.moved` has caught the card's own field up with a request that was just announced.
 */
export function pendingApprovalID(ctx: Ctx, id: CardKey): string | undefined {
  const card = cardOf(ctx, id);
  if (card?.approvalId) return card.approvalId;
  const waiting = (ctx.S.chat[id] ?? []).find(
    (x): x is ApprovalMsg => x.k === "approval" && x.st === "waiting",
  );
  return waiting?.approvalId;
}

/** Asks the daemon to answer one approval, and shows the one plain sentence a refusal carries. */
async function decide(
  ctx: Ctx,
  approvalId: string,
  decision: "approved" | "denied",
): Promise<boolean> {
  const api = ctx.env.data?.api;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return false;
  }
  try {
    await api.decideApproval(approvalId, { decision });
    platform().haptic("success");
    return true;
  } catch (error) {
    toast(ctx, error instanceof ApiError ? error.message : "Could not answer that request.");
    platform().haptic("error");
    return false;
  }
}

/**
 * Approves the approval a card is waiting on, the daemon's way (S8b). False means the card is not
 * waiting on one the daemon knows about, or the daemon refused the answer; either way nothing was
 * changed and, for a refusal, the reason has already been shown.
 */
export async function approveOnDaemon(ctx: Ctx, id: CardKey): Promise<boolean> {
  const approvalId = pendingApprovalID(ctx, id);
  if (!approvalId) return false;
  return decide(ctx, approvalId, "approved");
}

/** Denies the approval a card is waiting on, the daemon's way (S8b). */
export async function denyOnDaemon(ctx: Ctx, id: CardKey): Promise<boolean> {
  const approvalId = pendingApprovalID(ctx, id);
  if (!approvalId) return false;
  return decide(ctx, approvalId, "denied");
}

/** The card a card-scoped approval event names, found by the daemon's own id. */
function cardKeyOf(ctx: Ctx, daemonId: string): CardKey | undefined {
  return ctx.S.cards.find((one) => one.daemonId === daemonId)?.id;
}

/** Reads the fields of an announced approval this module needs; anything else is left for later. */
function approvalFieldsOf(value: unknown): { id: string; title: string; command: string } | null {
  if (!isRecord(value) || typeof value.id !== "string" || typeof value.title !== "string") {
    return null;
  }
  return {
    id: value.id,
    title: value.title,
    command: typeof value.command === "string" ? value.command : "",
  };
}

/**
 * Adds the block a live `approval.requested` draws to an open card's chat, so a person watching a
 * card sees the request the moment its agent makes it rather than only on the card's next open (the
 * same reason `applyToolCall` exists for a tool call). A card whose approval block is already
 * there - this event repeated, or the page read that opened the chat already carried it - is left
 * alone.
 */
function applyApprovalRequested(ctx: Ctx, event: WireEvent): void {
  const data = isRecord(event.data) ? event.data : null;
  const daemonId = data && typeof data.cardId === "string" ? data.cardId : "";
  const key = daemonId ? cardKeyOf(ctx, daemonId) : undefined;
  const approval = data ? approvalFieldsOf(data.approval) : null;
  if (!key || !approval) return;
  const chat = ctx.S.chat[key] ?? [];
  ctx.S.chat[key] = chat;
  if (chat.some((one) => one.k === "approval" && one.approvalId === approval.id)) return;
  chat.push({
    id: `s${takeMid(ctx.ids)}`,
    approvalId: approval.id,
    k: "approval",
    st: "waiting",
    cmd: approval.command || approval.title,
    why: approval.command ? approval.title : "",
  });
}

const isApprovalState = (state: unknown): state is ApprovalState =>
  state === "approved" || state === "denied";

/**
 * Rewrites an open card's approval block to how a live `approval.resolved` says it was answered,
 * found by the approval's own id (not the message's), so a card open in one view follows an answer
 * given in another - Home's needs-you card, or a second window - without a reload (N7's "every view
 * updates together").
 */
function applyApprovalResolved(ctx: Ctx, event: WireEvent): void {
  const data = isRecord(event.data) ? event.data : null;
  const daemonId = data && typeof data.cardId === "string" ? data.cardId : "";
  const approvalId = data && typeof data.approvalId === "string" ? data.approvalId : "";
  const state = data?.state;
  const key = daemonId ? cardKeyOf(ctx, daemonId) : undefined;
  if (!key || !approvalId || !isApprovalState(state)) return;
  const msg = (ctx.S.chat[key] ?? []).find(
    (one): one is ApprovalMsg => one.k === "approval" && one.approvalId === approvalId,
  );
  if (msg) msg.st = state;
}

/**
 * Applies one event of a card's own topic to its approval block, the live half of S8b: the paged
 * read of a card's chat (`sync/card-session.ts`'s `readOpenCard`) is what a chat opened after the
 * fact draws from, and this is what keeps it current while it stays open.
 */
export function applyApprovalEvent(ctx: Ctx, event: WireEvent): void {
  if (!approvalsOnDaemon(ctx)) return;
  if (event.type === EventTypeApprovalRequested) applyApprovalRequested(ctx, event);
  else if (event.type === EventTypeApprovalResolved) applyApprovalResolved(ctx, event);
}
