/**
 * The approval route of the fake daemon (section S8b, docs/backend-checklist.md B3.4, N7):
 * `POST /v1/approvals/{id}` answers one permission request an agent is blocked on. The route takes
 * the approval's own id, not a card's, so the row it changes is found by searching every card's
 * history for the one whose approval block carries it - the same reason `internal/history`'s real
 * `ResolveApproval` needs a query rather than a card-scoped lookup (internal/history/approvals.go):
 * this id is never a card's, and nothing here is keyed by it.
 *
 * The words and the order of the writes are the daemon's own (internal/api/routes_approvals.go,
 * internal/session/approval.go's Respond): the row's own state moves, the card leaves Needs you
 * when this was the reason it was there (`clearApprovalNeeds`), and `approval.resolved` is
 * announced on the card's own topic so every view of the same row follows it. Nothing here starts
 * a request the way a real agent does - the fake daemon has no agent turn to block - so a test sets
 * up the waiting row itself (`historyRow`, with an `approval` block and a matching `needsReason`)
 * the way it sets up any other fixture.
 */
import type { ChatApprovalState, DecideApprovalRequest, Card as WireCard } from "@marshal/protocol";
import { emptyAnswer, errorAnswer, type FakeRequest } from "~/data/testing/fake-fetch";
import {
  type CardStore,
  type HistoryRow,
  publishCard,
  type Route,
  STATUS,
} from "./fake-card-shared";

const alreadyAnswered = (): Response =>
  errorAnswer(STATUS.conflict, "conflict", "Somebody already answered that request.");

const noApproval = (): Response =>
  errorAnswer(STATUS.notFound, "not_found", "Marshal cannot find that approval.");

const badDecision = (): Response =>
  errorAnswer(STATUS.badRequest, "invalid_argument", "A decision must be approved or denied.");

/** The row of any card's history whose approval block carries this id, if one does. */
function approvalRowOf(store: CardStore, approvalId: string): HistoryRow | undefined {
  return store.history.find((row) => row.message.approval?.id === approvalId);
}

/** The body of an answer, or the daemon's own refusal for one that is not a decision. */
function readDecision(request: FakeRequest): DecideApprovalRequest | Response {
  if (!request.body) return badDecision();
  let body: DecideApprovalRequest;
  try {
    body = JSON.parse(request.body) as DecideApprovalRequest;
  } catch {
    return badDecision();
  }
  if (body.decision !== "approved" && body.decision !== "denied") return badDecision();
  return body;
}

const isRefusal = (value: DecideApprovalRequest | Response): value is Response =>
  value instanceof Response;

/**
 * Moves a card off Needs you once this was the approval it was waiting on, the way
 * `internal/session/approval.go`'s `clearApprovalNeeds` does: a card whose reason has since
 * changed to something else is left exactly as it is. Clears `needsReason` itself, not just the
 * state, the way the daemon's `SetState` now does whenever a card leaves needs (it did not
 * always: a card left "working" with its old reason, including the just-answered approval's own
 * id, still attached - `Card.approvalId` derives from `needsReason.approvalId`, so a resolved
 * approval kept answering as if it were still waiting).
 */
function clearNeeds(store: CardStore, card: WireCard): void {
  if (card.state !== "needs" || card.needsReason?.kind !== "approval-needed") return;
  card.state = "working";
  card.needsReason = null;
  card.updatedAt = store.now();
  publishCard(store, card, "card.moved", { from: "needs" });
}

/** POST /v1/approvals/{id}: answer a waiting approval. */
export function approvalRoute(
  { store, request, method }: Route,
  approvalId: string,
): Response | undefined {
  if (method !== "POST") return undefined;
  const row = approvalRowOf(store, approvalId);
  if (!row?.message.approval) return noApproval();
  if (row.message.approval.state !== "waiting") return alreadyAnswered();
  const decision = readDecision(request);
  if (isRefusal(decision)) return decision;
  const state: ChatApprovalState = decision.decision === "approved" ? "approved" : "denied";
  row.message.approval = { ...row.message.approval, state };
  if (row.activity) {
    row.activity = { ...row.activity, state: state === "approved" ? "ok" : "failed" };
  }
  const card = store.cards.find((one) => one.id === row.cardId);
  if (card) {
    clearNeeds(store, card);
    store.publish(`card:${card.id}`, "approval.resolved", {
      cardId: card.id,
      approvalId,
      state,
      decidedBy: "person",
      at: store.now(),
    });
  }
  return emptyAnswer();
}
