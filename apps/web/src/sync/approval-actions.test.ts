import type { Card as WireCard, Event as WireEvent } from "@marshal/protocol";
import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import type { Marshal } from "~/mock";
import type { Ctx } from "~/mock/context";
import type { Card } from "~/mock/types";
import * as platformModule from "~/platform";
import { cardId, type HistoryRow, historyRow, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createSyncedMarshal } from "~/testing/test-store";
import {
  applyApprovalEvent,
  approveOnDaemon,
  denyOnDaemon,
  pendingApprovalID,
} from "./approval-actions";

/*
 * The approval flow's own writes once approvals are the daemon's (section S8b, B3.4): the two
 * answers to a request an agent is blocked on, and the live approval.requested/approval.resolved
 * pair that follows one. The daemon owns where the request stands, so each answer is one route call
 * with the approval's own id, and the card it leaves Needs you for is the card the daemon's own
 * card.moved says, not a guess of its own.
 */

const APPROVALS_ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const, S8b: "daemon" as const };

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

const CARD = cardId(44);
const APPROVAL_ID = "app_01JQZ0000000000000000000AD";
const APPROVE = `POST /v1/approvals/${encodeURIComponent(APPROVAL_ID)}`;

const api44 = (fields: Partial<WireCard> = {}): WireCard =>
  wireCard({
    projectId: "api",
    number: 44,
    title: "Upgrade grpc-go",
    state: "needs",
    needsReason: {
      kind: "approval-needed",
      text: "Approval needed to run rm -rf build.",
      approvalId: APPROVAL_ID,
    },
    ...fields,
  });

/** The stored approval message a card waits on, which is what the route answers for. */
const approvalRow = (fields: { state?: "waiting" | "approved" | "denied" } = {}): HistoryRow =>
  historyRow(CARD, {
    id: "01M3APPROVAL00000000000AAA",
    kind: "approval",
    seq: 7,
    text: "Asked to run rm -rf build",
    approval: {
      id: APPROVAL_ID,
      state: fields.state ?? "waiting",
      command: "rm -rf build",
      reason: "Clean the build folder",
    },
  });

interface Fixture {
  M: Marshal;
  ctx: Ctx;
  d: FakeDaemon;
}

/** A store that follows a fake daemon holding one card waiting on an approval. */
async function setup(
  cards: readonly WireCard[] = [api44()],
  history: readonly HistoryRow[] = [approvalRow()],
): Promise<Fixture> {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards: [...cards], history });
  daemon = d;
  const M = await createSyncedMarshal(d, { sections: APPROVALS_ON_DAEMON });
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

describe("finding the approval a card is waiting on", () => {
  it("reads it from the card itself, so a card nobody has opened still answers (S8b's Home gap)", async () => {
    const { ctx } = await setup();
    expect(pendingApprovalID(ctx, "api#44")).toBe(APPROVAL_ID);
  });
});

describe("answering an approval on the daemon", () => {
  it("approves it in one call, and moves the card the daemon's card.moved says to", async () => {
    const { M, ctx, d } = await setup();
    expect(await approveOnDaemon(ctx, "api#44")).toBe(true);
    expect(d.routes()).toContain(APPROVE);
    expect(d.bodies(APPROVE)).toEqual([{ decision: "approved" }]);
    expect(storeCard(M, "api#44").state).toBe("working");
    expect(storeCard(M, "api#44").approvalId).toBeUndefined();
  });

  it("denies it in one call", async () => {
    const { M, d, ctx } = await setup();
    expect(await denyOnDaemon(ctx, "api#44")).toBe(true);
    expect(d.bodies(APPROVE)).toEqual([{ decision: "denied" }]);
    expect(storeCard(M, "api#44").state).toBe("working");
  });

  it("taps the phone for an answer that went through, and marks one that was refused", async () => {
    const haptic = vi.fn();
    vi.spyOn(platformModule, "platform").mockReturnValue({
      ...platformModule.createPlatform("web"),
      haptic,
    });
    const first = await setup();
    await approveOnDaemon(first.ctx, "api#44");
    expect(haptic).toHaveBeenLastCalledWith("success");
    const second = await setup(undefined, [approvalRow({ state: "approved" })]);
    await approveOnDaemon(second.ctx, "api#44");
    expect(haptic).toHaveBeenLastCalledWith("error");
  });

  it("does nothing when the card is not waiting on one the daemon knows about", async () => {
    const { ctx, d } = await setup([api44({ state: "working", needsReason: null })], []);
    expect(await approveOnDaemon(ctx, "api#44")).toBe(false);
    expect(await denyOnDaemon(ctx, "api#44")).toBe(false);
    expect(d.routes()).not.toContain(APPROVE);
  });

  it("says so, for Approve and for Deny, instead of doing nothing", async () => {
    const { M, ctx } = await setup([api44({ state: "working", needsReason: null })], []);
    await approveOnDaemon(ctx, "api#44");
    await denyOnDaemon(ctx, "api#44");
    expect(toasts(M)).toEqual([
      "That request is not waiting for an answer any more.",
      "That request is not waiting for an answer any more.",
    ]);
  });

  it("shows the daemon's sentence when the request was already answered", async () => {
    const { M, ctx } = await setup(undefined, [approvalRow({ state: "approved" })]);
    expect(await approveOnDaemon(ctx, "api#44")).toBe(false);
    expect(toasts(M)).toEqual(["Somebody already answered that request."]);
  });
});

describe("a live approval event on an open card", () => {
  const eventOf = (type: string, data: unknown): WireEvent => ({
    seq: 1,
    topic: `card:${CARD}`,
    type: type as WireEvent["type"],
    at: "2026-09-27T12:00:00.000Z",
    data,
  });

  it("adds the block a person has not opened the card to see yet", async () => {
    const { ctx } = await setup(undefined, []);
    applyApprovalEvent(
      ctx,
      eventOf("approval.requested", {
        cardId: CARD,
        approval: { id: "app_new", title: "Run go mod tidy", command: "go mod tidy" },
      }),
    );
    const chat = ctx.S.chat["api#44"] ?? [];
    const added = chat.find((one) => one.k === "approval" && one.approvalId === "app_new");
    expect(added).toMatchObject({ k: "approval", st: "waiting", cmd: "go mod tidy" });
  });

  it("rewrites an open chat's approval block when it is answered elsewhere", async () => {
    const { ctx } = await setup();
    // The chat is opened by reading it once, the way `readOpenCard` would.
    ctx.S.chat["api#44"] = [
      {
        id: "m1",
        approvalId: APPROVAL_ID,
        k: "approval",
        st: "waiting",
        cmd: "rm -rf build",
        why: "",
      },
    ];
    applyApprovalEvent(
      ctx,
      eventOf("approval.resolved", { cardId: CARD, approvalId: APPROVAL_ID, state: "denied" }),
    );
    const msg = ctx.S.chat["api#44"]?.find((one) => one.k === "approval");
    expect(msg).toMatchObject({ st: "denied" });
  });
});
