/**
 * The plan routes of one card in the fake daemon (section S8c, docs/backend-checklist.md B5.2):
 * the plan a card waits on in plan-first mode, and the three answers to it — approve, reject, and
 * edit.
 *
 * The words, the order of the writes, and the events published are the daemon's own
 * (internal/api/routes_plan.go, internal/session/plan.go), so a test drives the same refusal a
 * person sees and hears the same `plan.updated`. A plan is a message in the card's chat and not a
 * table of its own, so the newest plan message a card holds is the one a person is answering.
 *
 * One stored event is both a chat message and an activity item (internal/history/wire.go): the line
 * an answer adds is drawn in the card's chat and in its activity list, and only the one that ended
 * somehow — that is, the one that carries a state — is an activity item at all.
 */
import type {
  ActivityItem,
  ChatMessage,
  ChatPlan,
  EditPlanRequest,
  Card as WireCard,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { type CardStore, type HistoryRow, publishCard, STATUS } from "./fake-card-shared";

/** How many plan messages this fake daemon has written, so no two of them share an id. */
let written = 0;

/** How many digits of that count an id carries, so two ids differ from the first plan on. */
const ID_DIGITS = 5;

/** The opaque id of a message the fake daemon writes. */
function freshEventId(): string {
  written += 1;
  return `01M3PLAN0000000000000${String(written).padStart(ID_DIGITS, "0")}`;
}

/** The one line a plan message adds to the history (internal/history/agents.go, planSummary). */
function planSummary(steps: number): string {
  return steps === 1 ? "Plan with 1 step" : `Plan with ${steps} steps`;
}

/**
 * The line an edit leaves. The count is in the line itself because the daemon only gives an activity
 * row a second line when it stands for a stored tool call, and an edit is a system note rather than
 * one (internal/session/plan.go, planEditSummary).
 */
function planEditSummary(steps: number): string {
  return steps === 1 ? "You edited the plan to 1 step" : `You edited the plan to ${steps} steps`;
}

/** The card's own topic, where a plan change is announced (internal/session/plan.go, announcePlan). */
const cardTopic = (cardId: string): string => `card:${cardId}`;

/** The newest plan message a card holds, which is the plan a person is answering. */
function newestPlan(store: CardStore, cardId: string): HistoryRow | undefined {
  let newest: HistoryRow | undefined;
  for (const row of store.history) {
    if (row.cardId !== cardId || !row.message.plan) continue;
    if (!newest || row.message.seq > newest.message.seq) newest = row;
  }
  return newest;
}

/** The place the next record takes in a card's history: after every row it already holds. */
function nextSeq(store: CardStore, cardId: string): number {
  const seqs = store.history.filter((row) => row.cardId === cardId).map((row) => row.message.seq);
  return Math.max(0, ...seqs) + 1;
}

/** The daemon's own words for a card with no plan (protocol.NotFound("plan")). */
const noPlan = (): Response =>
  errorAnswer(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that plan. It may have been removed.",
  );

/** The daemon's own words for a plan a person has already answered. */
const alreadyAnswered = (): Response =>
  errorAnswer(STATUS.conflict, "conflict", "That plan has already been answered.");

const isRefusal = (value: ChatPlan | Response): value is Response => value instanceof Response;

/**
 * The plan a card is waiting on, or the answer the daemon refuses with. A plan an agent wrote and
 * one a person has already edited both still wait for an answer.
 */
function answerablePlan(store: CardStore, card: WireCard): ChatPlan | Response {
  const plan = newestPlan(store, card.id)?.message.plan;
  if (!plan) return noPlan();
  if (plan.state !== "waiting" && plan.state !== "edited") return alreadyAnswered();
  return plan;
}

/** Stores a plan as the card's newest plan message, and answers the row it was written as. */
function storePlan(store: CardStore, card: WireCard, plan: ChatPlan): HistoryRow {
  const row: HistoryRow = {
    cardId: card.id,
    message: {
      id: freshEventId(),
      kind: "plan",
      seq: nextSeq(store, card.id),
      at: store.now(),
      text: planSummary(plan.steps.length),
      tool: null,
      diff: null,
      plan,
      approval: null,
      card: null,
    },
    activity: null,
  };
  store.history.push(row);
  return row;
}

/**
 * The line one answer adds: a chat message and, because it ended, an activity item. An answer that
 * is an approval is drawn as an approval; an edit is a system note, which the activity list draws
 * as any other tool row (internal/history/wire.go, ActivityKindOf). Neither carries a second line:
 * only a stored tool call has the detail a result is read from.
 */
function storeAnswer(
  store: CardStore,
  card: WireCard,
  answer: {
    text: string;
    kind: ChatMessage["kind"];
    activity: ActivityItem["kind"];
    state: ActivityItem["state"];
  },
): void {
  const id = freshEventId();
  const seq = nextSeq(store, card.id);
  const at = store.now();
  // The approval block of a plan answer carries no command: the state is where the plan stands, and
  // the one line that says what happened is the message's own text.
  const message: ChatMessage = {
    id,
    kind: answer.kind,
    seq,
    at,
    text: answer.text,
    tool: null,
    diff: null,
    plan: null,
    approval:
      answer.kind === "approval"
        ? { id: freshEventId(), state: "approved", command: "", reason: "" }
        : null,
    card: null,
  };
  store.history.push({
    cardId: card.id,
    message,
    activity: {
      id,
      kind: answer.activity,
      seq,
      at,
      text: answer.text,
      result: "",
      state: answer.state,
    },
  });
}

/** Announces the plan a card now holds, so every view of it follows one answer without a re-read. */
function announce(store: CardStore, card: WireCard, row: HistoryRow, plan: ChatPlan): void {
  store.publish(cardTopic(card.id), "plan.updated", {
    cardId: card.id,
    messageId: row.message.id,
    plan,
    at: store.now(),
  });
}

/** Writes the card fields an answer changes, and says so the way a card edit does. */
function writeCard(store: CardStore, card: WireCard, change: Partial<WireCard>): void {
  Object.assign(card, change);
  card.updatedAt = store.now();
  publishCard(store, card, "card.updated");
}

/** Moves the card, and says so the way a move does: the column it came from is in the event. */
function moveCard(store: CardStore, card: WireCard, to: WireCard["state"]): void {
  if (card.state === to) return;
  const from = card.state;
  card.state = to;
  card.needsReason = null;
  card.updatedAt = store.now();
  publishCard(store, card, "card.moved", { from });
}

/**
 * POST /v1/cards/{id}/plan/approve: the plan stops waiting, the card starts working on it, and it
 * leaves plan-only, which is what lets the agent change files rather than plan them. The "doing
 * now" line is the plan's own first step, unless the card already said what it was doing.
 */
function approvePlan(store: CardStore, card: WireCard): Response {
  const plan = answerablePlan(store, card);
  if (isRefusal(plan)) return plan;
  const approved: ChatPlan = { ...plan, state: "approved" };
  const row = storePlan(store, card, approved);
  storeAnswer(store, card, {
    text: "You approved the plan",
    kind: "approval",
    activity: "approval",
    state: "ok",
  });
  announce(store, card, row, approved);
  const first = approved.steps[0] ?? "";
  writeCard(store, card, {
    permissionMode: "auto-edits",
    doingNow: card.doingNow === "" && first ? first : "Starting work on the plan",
  });
  moveCard(store, card, "working");
  return jsonAnswer(card);
}

/** POST /v1/cards/{id}/plan/reject: the plan is sent back, and the card returns to planning. */
function rejectPlan(store: CardStore, card: WireCard): Response {
  const plan = answerablePlan(store, card);
  if (isRefusal(plan)) return plan;
  const rejected: ChatPlan = { ...plan, state: "rejected" };
  const row = storePlan(store, card, rejected);
  storeAnswer(store, card, {
    text: "You rejected the plan",
    kind: "approval",
    activity: "approval",
    state: "failed",
  });
  announce(store, card, row, rejected);
  writeCard(store, card, { doingNow: "Reworking the plan" });
  moveCard(store, card, "planning");
  return jsonAnswer(card);
}

/** The steps a person left, with the blanks dropped, or null when there is none left to plan with. */
function cleanSteps(steps: readonly string[] | undefined): string[] | null {
  const cleaned = (steps ?? []).map((step) => step.trim()).filter(Boolean);
  return cleaned.length > 0 ? cleaned : null;
}

/**
 * PUT /v1/cards/{id}/plan: the steps are replaced and the plan stays waiting, so the card is not
 * moved and nothing about it changes. Only the steps change: the files, the risks, and the checks
 * are the agent's own reading of the work.
 */
function editPlan(store: CardStore, card: WireCard, request: FakeRequest): Response {
  const body = readEditRequest(request);
  if (body instanceof Response) return body;
  const steps = cleanSteps(body.steps);
  if (!steps) {
    return errorAnswer(STATUS.badRequest, "invalid_argument", "A plan needs at least one step.");
  }
  const plan = answerablePlan(store, card);
  if (isRefusal(plan)) return plan;
  const edited: ChatPlan = { ...plan, state: "edited", steps };
  const row = storePlan(store, card, edited);
  storeAnswer(store, card, {
    text: planEditSummary(steps.length),
    kind: "system",
    activity: "tool",
    state: "ok",
  });
  announce(store, card, row, edited);
  return jsonAnswer(card);
}

/** The body of an edit, or the daemon's own answer for a body that is not one. */
function readEditRequest(request: FakeRequest): EditPlanRequest | Response {
  if (!request.body) {
    return errorAnswer(
      STATUS.badRequest,
      "invalid_argument",
      "The request body is empty. Send a JSON object.",
    );
  }
  try {
    return JSON.parse(request.body) as EditPlanRequest;
  } catch {
    return errorAnswer(
      STATUS.badRequest,
      "invalid_argument",
      "The request body is not valid JSON. Check it and try again.",
    );
  }
}

/**
 * One request under `/v1/cards/{id}/plan`: `approve` and `reject` are POSTs to a named route of
 * their own, and the edit is a PUT of the plan itself.
 */
export function planRoute(
  store: CardStore,
  card: WireCard,
  request: FakeRequest,
  method: string,
  extra: string | undefined,
): Response | undefined {
  if (method === "PUT") return extra ? undefined : editPlan(store, card, request);
  if (method !== "POST") return undefined;
  if (extra === "approve") return approvePlan(store, card);
  if (extra === "reject") return rejectPlan(store, card);
  return undefined;
}
