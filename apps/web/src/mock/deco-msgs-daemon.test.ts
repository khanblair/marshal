import { type ChatPlan, EventTypePlanUpdated, type Card as WireCard } from "@marshal/protocol";
import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import { cardId, historyRow, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { createSyncedMarshal } from "~/testing/test-store";
import type { CardKey } from "./card-key";
import type { MsgView } from "./deco-msgs";
import type { Marshal } from "./marshal";
import type { Msg } from "./types";

/*
 * The plan block once the plans are the daemon's (section S8c), seen through the view model the
 * screen actually draws. The three answers the block's buttons make are the daemon's routes, and the
 * `plan.updated` it publishes while the card is open is what redraws the block. The mock's own plan
 * block is covered beside this file, in `deco-msgs.test.ts`, which pins S8c to the mock.
 */

const PLAN_ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const, S8c: "daemon" as const };
const CARD = cardId(41);

const route = (method: string, action = ""): string =>
  `${method} /v1/cards/${encodeURIComponent(CARD)}${action ? `/${action}` : ""}`;
const APPROVE = route("POST", "plan/approve");
const REJECT = route("POST", "plan/reject");
const EDIT = route("PUT", "plan");

const STEPS = ["Read the refresh path", "Add the retry"];
const FILES = ["internal/auth/refresh.go"];

const api41 = (): WireCard =>
  wireCard({
    projectId: "api",
    number: 41,
    title: "Fix token refresh on login",
    state: "planning",
  });

const plan = (fields: Partial<ChatPlan> = {}): ChatPlan => ({
  state: "waiting",
  steps: [...STEPS],
  files: [...FILES],
  risks: [],
  checks: [],
  ...fields,
});

/** The stored plan the daemon serves for the card, and the block a person answers. */
const PLAN_ID = "01M3PLAN000000000000000AAA";
const planRow = () =>
  historyRow(CARD, {
    id: PLAN_ID,
    kind: "plan",
    seq: 6,
    text: "Plan with 2 steps",
    plan: plan(),
  });

/** The same plan in the shape the store keeps a chat in, for the block a test answers. */
const planMsg = (fields: Partial<ChatPlan> = {}): Msg => {
  const one = plan(fields);
  return {
    id: PLAN_ID,
    k: "plan",
    st: one.state === "approved" ? "approved" : one.state === "rejected" ? "rejected" : "waiting",
    editing: false,
    steps: [...one.steps],
    files: [...one.files],
    risks: [...one.risks],
    checks: [...one.checks],
  };
};

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

/** A store following a fake daemon, with the card's chat holding the plan a person is answering. */
async function setup(): Promise<{ M: Marshal; d: FakeDaemon }> {
  const d = createFakeDaemon({
    projects: PROTOTYPE_PROJECTS,
    cards: [api41()],
    history: [planRow()],
  });
  daemon = d;
  const M = await createSyncedMarshal(d, { sections: PLAN_ON_DAEMON });
  M.S.chat["api#41"] = [planMsg()];
  return { M, d };
}

const planView = (M: Marshal, id: CardKey = "api#41"): MsgView => {
  const view = M.decoMsgs(M.S.chat[id] ?? [], id).find((one) => one.isPlan);
  if (!view) throw new Error("that chat has no plan block");
  return view;
};

const events = (M: Marshal): string[] => M.S.toasts.map((toast) => toast.msg);

describe("the plan block on the daemon", () => {
  it("approves through the daemon's route, and draws the card it answers with", async () => {
    const { M, d } = await setup();
    planView(M).approve?.();
    await vi.waitFor(() => expect(d.routes()).toContain(APPROVE));
    // The block's own button is fire-and-forget like the card's action row, so the card it moves is
    // waited for: the store draws the daemon's answer, which left planning and plan-only.
    await vi.waitFor(() => expect(M.card("api#41")?.state).toBe("working"));
    expect(M.card("api#41")?.perm).toBe("Auto-accept edits");
    expect(M.card("api#41")?.doing).toBe(STEPS[0]);
  });

  it("rejects through the daemon's route, and the card goes back to planning", async () => {
    const { M, d } = await setup();
    planView(M).reject?.();
    await vi.waitFor(() => expect(d.routes()).toContain(REJECT));
    await vi.waitFor(() => expect(M.card("api#41")?.doing).toBe("Reworking the plan"));
    expect(M.card("api#41")?.state).toBe("planning");
  });

  it("saves the editor's steps to the daemon, one per line", async () => {
    const { M, d } = await setup();
    planView(M).edit?.();
    expect(planView(M)).toMatchObject({ editing: true, notEditing: false });
    const form = document.createElement("form");
    const steps = document.createElement("textarea");
    steps.name = "steps";
    steps.value = "One\nTwo";
    form.append(steps);
    form.addEventListener("submit", (e) => planView(M).save?.(e));
    form.dispatchEvent(new SubmitEvent("submit", { cancelable: true }));
    await vi.waitFor(() => expect(d.bodies(EDIT)).toEqual([{ steps: ["One", "Two"] }]));
  });

  it("hears plan.updated on the card's topic and redraws the block it carries", async () => {
    const { M, d } = await setup();
    M.openCard("api#41");
    // Opening the card follows its own topic and reads its chat back, so the plan the daemon holds
    // is the one the block draws before anything changes.
    await vi.waitFor(() => expect(planView(M).id).toBe(PLAN_ID));
    d.emit(`card:${CARD}`, EventTypePlanUpdated, {
      cardId: CARD,
      messageId: "new-plan",
      plan: plan({ state: "approved" }),
      at: "2026-09-27T12:00:00.000Z",
    });
    await vi.waitFor(() => expect(planView(M).id).toBe("new-plan"));
    // The plan was replaced, not edited in place, and the chat still holds one plan line.
    expect((M.S.chat["api#41"] ?? []).filter((one) => one.k === "plan")).toHaveLength(1);
    expect(planView(M)).toMatchObject({ statusLabel: "Approved", done: true });
  });

  it("shows the daemon's sentence through the block when the answer is refused", async () => {
    const { M, d } = await setup();
    d.refuseNext(APPROVE, 409, "conflict", "That plan has already been answered.");
    planView(M).approve?.();
    await vi.waitFor(() => expect(events(M)).toContain("That plan has already been answered."));
    expect(M.card("api#41")?.state).toBe("planning");
  });
});
