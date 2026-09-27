import {
  type ChatPlan,
  EventTypePlanUpdated,
  type Card as WireCard,
  type Event as WireEvent,
} from "@marshal/protocol";
import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import type { Marshal } from "~/mock";
import type { Ctx } from "~/mock/context";
import type { Card, Msg } from "~/mock/types";
import { cardId, type HistoryRow, historyRow, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createSyncedMarshal, createTestMarshal } from "~/testing/test-store";
import { planBlock } from "./chat-mapper";
import { applyPlanUpdated, approvePlan, rejectPlan, savePlan } from "./plan-actions";

/*
 * The plan's own writes once the plans are the daemon's (section S8c): the three answers to the plan
 * a card waits on, and the `plan.updated` that follows each of them. The daemon owns where a plan
 * stands, so every one of these is one route call and the card it answers with; the refusal a test
 * injects is shown in the daemon's own words, and nothing in the store is touched.
 */

/** The sections with the cards and the plans on the daemon, which is what the cutover says. */
const PLAN_ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const, S8c: "daemon" as const };

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

const CARD = cardId(41);

/** The address of one of the card's plan routes, as the client sends it. */
const route = (method: string, action = ""): string =>
  `${method} /v1/cards/${encodeURIComponent(CARD)}${action ? `/${action}` : ""}`;
const APPROVE = route("POST", "plan/approve");
const REJECT = route("POST", "plan/reject");
const EDIT = route("PUT", "plan");

const api41 = (fields: Partial<WireCard> = {}): WireCard =>
  wireCard({
    projectId: "api",
    number: 41,
    title: "Fix token refresh on login",
    state: "planning",
    ...fields,
  });

const STEPS = ["Read the refresh path", "Add the retry", "Cover it with a test"];

/** A plan as the daemon stores it, which is what a card's chat holds. */
const plan = (fields: Partial<ChatPlan> = {}): ChatPlan => ({
  state: "waiting",
  steps: [...STEPS],
  files: ["internal/auth/refresh.go"],
  risks: ["the login flow"],
  checks: ["go test ./internal/auth/"],
  ...fields,
});

/** The stored plan message a card waits on, which is what its routes answer for. */
const planRow = (fields: Partial<ChatPlan> = {}): HistoryRow =>
  historyRow(CARD, {
    id: "01M3PLAN000000000000000AAA",
    kind: "plan",
    seq: 6,
    text: "Plan with 3 steps",
    plan: plan(fields),
  });

interface Fixture {
  M: Marshal;
  ctx: Ctx;
  d: FakeDaemon;
}

/** A store that follows a fake daemon holding one planning card with a plan waiting on it. */
async function setup(history: readonly HistoryRow[] = [planRow()]): Promise<Fixture> {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards: [api41()], history });
  daemon = d;
  const M = await createSyncedMarshal(d, { sections: PLAN_ON_DAEMON });
  await settled(d);
  return { M, ctx: contextOf(M), d };
}

/** Waits until the app has stopped asking for boards, so a late snapshot cannot put a card back. */
async function settled(d: FakeDaemon): Promise<void> {
  let last = -1;
  await vi.waitFor(() => {
    const loads = d.routes().filter((one) => one.endsWith("/board")).length;
    const steady = loads > 0 && loads === last;
    last = loads;
    expect(steady).toBe(true);
  });
}

function storeCard(M: Marshal, id: string): Card {
  const card = M.card(id);
  if (!card) throw new Error(`the store has no card ${id}`);
  return card;
}

const toasts = (M: Marshal): string[] => M.S.toasts.map((toast) => toast.msg);

/** One event as the stream delivers it, for the tests that apply the daemon's own a second time. */
const eventOf = (type: string, data: unknown): WireEvent => ({
  seq: 1,
  topic: `card:${CARD}`,
  type: type as WireEvent["type"],
  at: "2026-09-27T12:00:00.000Z",
  data,
});

describe("answering a plan on the daemon", () => {
  it("approves it in one call, and keeps the card the daemon answered with", async () => {
    const { M, ctx, d } = await setup();
    expect(await approvePlan(ctx, "api#41")).toBe(true);
    expect(d.routes()).toContain(APPROVE);
    // The daemon moved the card out of planning and out of plan-only, which is what its own answer
    // says; the store draws that card, not a guess of its own.
    expect(storeCard(M, "api#41").state).toBe("working");
    expect(storeCard(M, "api#41").perm).toBe("Auto-accept edits");
    expect(storeCard(M, "api#41").doing).toBe(STEPS[0]);
    expect(toasts(M)).toEqual(["Plan approved"]);
  });

  it("rejects it in one call, and the card goes back to planning", async () => {
    const { M, ctx, d } = await setup();
    expect(await rejectPlan(ctx, "api#41")).toBe(true);
    expect(d.routes()).toContain(REJECT);
    expect(storeCard(M, "api#41").state).toBe("planning");
    expect(storeCard(M, "api#41").doing).toBe("Reworking the plan");
    expect(toasts(M)).toEqual(["Plan rejected"]);
  });

  it("saves the steps a person left, one per line, and leaves the plan where it was", async () => {
    const { M, ctx, d } = await setup();
    expect(await savePlan(ctx, "api#41", "one\ntwo")).toBe(true);
    expect(d.bodies(EDIT)).toEqual([{ steps: ["one", "two"] }]);
    // An edit is not an answer: the plan stays waiting and the card is not moved.
    expect(storeCard(M, "api#41").state).toBe("planning");
    expect(storeCard(M, "api#41").doing).toBe("");
    expect(toasts(M)).toEqual(["Plan saved"]);
  });

  it("sends a blank line through, so the daemon is the one that drops it", async () => {
    const { ctx, d } = await setup();
    expect(await savePlan(ctx, "api#41", "one\n\n  \ntwo")).toBe(true);
    expect(d.bodies(EDIT)).toEqual([{ steps: ["one", "", "  ", "two"] }]);
  });
});

describe("a plan the daemon refuses", () => {
  it("shows the daemon's sentence and changes nothing when the card has no plan", async () => {
    const { M, ctx, d } = await setup([]);
    expect(await approvePlan(ctx, "api#41")).toBe(false);
    expect(d.routes()).toContain(APPROVE);
    expect(toasts(M)).toEqual(["Marshal cannot find that plan. It may have been removed."]);
    expect(storeCard(M, "api#41").state).toBe("planning");
  });

  it("shows the daemon's sentence when the plan was already answered", async () => {
    const { M, ctx, d } = await setup([planRow({ state: "approved" })]);
    expect(await rejectPlan(ctx, "api#41")).toBe(false);
    expect(d.routes()).toContain(REJECT);
    expect(toasts(M)).toEqual(["That plan has already been answered."]);
    expect(storeCard(M, "api#41").state).toBe("planning");
  });

  it("shows the daemon's sentence when an edit leaves no step at all", async () => {
    const { M, ctx, d } = await setup();
    expect(await savePlan(ctx, "api#41", "   \n\n")).toBe(false);
    expect(d.bodies(EDIT)).toEqual([{ steps: ["   ", "", ""] }]);
    expect(toasts(M)).toEqual(["A plan needs at least one step."]);
  });
});

describe("plan.updated", () => {
  it("replaces the newest plan the chat holds with the one the event carries", async () => {
    const { ctx } = await setup();
    const said: Msg = { id: "m1", k: "agent", text: "here is the plan" };
    const before = planBlock("old-plan", plan());
    ctx.S.chat["api#41"] = [said, before];
    applyPlanUpdated(
      ctx,
      eventOf(EventTypePlanUpdated, {
        cardId: CARD,
        messageId: "new-plan",
        plan: plan({ state: "approved" }),
        at: "2026-09-27T12:00:00.000Z",
      }),
    );
    const chat = ctx.S.chat["api#41"] ?? [];
    // The daemon replaces a plan rather than editing it in place, so the chat holds one plan still:
    // the newest, in the place the one it replaced had, and the line before it is untouched.
    expect(chat).toHaveLength(2);
    expect(chat[0]).toEqual(said);
    expect(chat[1]).toMatchObject({ id: "new-plan", k: "plan", st: "approved" });
  });

  it("adds the plan when the chat holds none, and ignores a card it does not have", async () => {
    const { ctx } = await setup();
    applyPlanUpdated(
      ctx,
      eventOf(EventTypePlanUpdated, {
        cardId: CARD,
        messageId: "first-plan",
        plan: plan({ state: "edited", steps: ["only one"] }),
        at: "2026-09-27T12:00:00.000Z",
      }),
    );
    expect(ctx.S.chat["api#41"]).toHaveLength(1);
    // A wire "edited" is a plan that still waits: it is flagged so the block says so, not answered.
    expect(ctx.S.chat["api#41"]?.[0]).toMatchObject({
      id: "first-plan",
      st: "waiting",
      edited: true,
    });
    applyPlanUpdated(
      ctx,
      eventOf(EventTypePlanUpdated, {
        cardId: cardId(9999),
        messageId: "ghost",
        plan: plan(),
        at: "2026-09-27T12:00:00.000Z",
      }),
    );
    expect(ctx.S.chat[cardId(9999)]).toBeUndefined();
  });
});

describe("a store whose plans are still the mock's", () => {
  it("applies no plan.updated, because the mock's own plan is the one it draws", () => {
    const M = createTestMarshal({ sections: { ...sectionStatus, S8a: "mock", S8c: "mock" } });
    const ctx = contextOf(M);
    const before = ctx.S.chat["api#41"]?.length ?? 0;
    applyPlanUpdated(
      ctx,
      eventOf(EventTypePlanUpdated, {
        cardId: cardId(41),
        messageId: "ignored",
        plan: plan(),
        at: "2026-09-27T12:00:00.000Z",
      }),
    );
    expect(ctx.S.chat["api#41"]?.length ?? 0).toBe(before);
    expect(ctx.S.chat["api#41"]?.some((msg) => msg.id === "ignored")).toBe(false);
  });
});

describe("the plan writes with no daemon", () => {
  it("says so plainly, and changes nothing, when the card's daemon is not there", async () => {
    const M = createTestMarshal({ sections: PLAN_ON_DAEMON });
    const ctx = contextOf(M);
    const card = M.card("api#41");
    if (!card) throw new Error("the store has no api#41 card");
    const before = { ...card };
    expect(await approvePlan(ctx, "api#41")).toBe(false);
    expect(await rejectPlan(ctx, "api#41")).toBe(false);
    expect(await savePlan(ctx, "api#41", "one")).toBe(false);
    expect(toasts(M)).toEqual([NOT_CONNECTED, NOT_CONNECTED, NOT_CONNECTED]);
    expect(card).toEqual(before);
  });

  it("asks nothing for a card the mock made, which has no daemon id", async () => {
    const M = createTestMarshal({ sections: PLAN_ON_DAEMON });
    const ctx = contextOf(M);
    const card = M.card("api#41");
    if (!card) throw new Error("the store has no api#41 card");
    card.daemonId = undefined;
    expect(await approvePlan(ctx, "api#41")).toBe(false);
    expect(toasts(M)).toEqual([]);
  });
});
