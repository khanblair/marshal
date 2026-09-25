import { type CardKey, cardLabel } from "../card-key";
import type { Ctx } from "../context";
import { addAct, feed, runTool, seq, setState, streamCard, toast } from "../engine";
import { card } from "../selectors";
import type { ApprovalMsg, ApprovalState, PlanMsg } from "../types";

const waitingApproval = (ctx: Ctx, id: CardKey): ApprovalMsg | undefined =>
  (ctx.S.chat[id] ?? []).find((x): x is ApprovalMsg => x.k === "approval" && x.st === "waiting");

const waitingPlan = (ctx: Ctx, id: CardKey): PlanMsg | undefined =>
  (ctx.S.chat[id] ?? []).find((x): x is PlanMsg => x.k === "plan" && x.st === "waiting");

/** Mirrors an approval decision into the project chats that asked for the same card. */
function syncProjectApproval(ctx: Ctx, id: CardKey, st: ApprovalState): void {
  for (const list of Object.values(ctx.S.chats)) {
    for (const ch of list) {
      for (const x of ch.msgs) {
        if (x.k === "approval" && x.cardId === id && x.st === "waiting") x.st = st;
      }
    }
  }
}

/* The scripted follow-up of approving the grpc-go upgrade on #44. */
const UPGRADE_RUN_MS = 2400;
const EDIT_CALL_SITES = { afterMs: 800, ms: 1200 };
const RUN_TESTS = { afterMs: 1600, ms: 3500 };

function afterUpgrade(ctx: Ctx, id: CardKey): void {
  const c = card(ctx, id);
  if (!c) return;
  seq([
    {
      afterMs: EDIT_CALL_SITES.afterMs,
      run: () =>
        runTool(ctx, id, {
          icon: "file-pen",
          action: "Edited internal/upstream/conn.go",
          result: "+4 −4",
          ms: EDIT_CALL_SITES.ms,
        }),
    },
    {
      afterMs: RUN_TESTS.afterMs,
      run: () => {
        c.doing = "Running go test ./...";
        runTool(ctx, id, {
          icon: "terminal",
          action: "Ran go test ./...",
          result: "229 passed",
          ms: RUN_TESTS.ms,
          kind: "test",
          done: () => {
            c.doing = "Committing the upgrade";
          },
        });
      },
    },
  ]);
}

/** Approves the card's waiting command, or its waiting plan when there is no command. */
export function approve(ctx: Ctx, id: CardKey): void {
  const c = card(ctx, id);
  const a = waitingApproval(ctx, id);
  if (!a) {
    if (waitingPlan(ctx, id)) approvePlan(ctx, id);
    return;
  }
  if (!c) return;
  a.st = "approved";
  syncProjectApproval(ctx, id, "approved");
  addAct(ctx, id, { kind: "approval", text: `You approved: ${a.cmd}`, result: "Approved" });
  feed(ctx, {
    kind: "approval",
    text: `You approved ${a.cmd} on ${cardLabel(c)}`,
    pid: c.p,
    cardId: id,
  });
  c.doing = `Running ${a.cmd}`;
  setState(ctx, c, "working");
  toast(ctx, "Approved");
  runTool(ctx, id, {
    icon: "terminal",
    action: `Ran ${a.cmd}`,
    result: "grpc v1.64.1 => v1.66.0",
    ms: UPGRADE_RUN_MS,
    kind: "command",
    done: () =>
      streamCard(
        ctx,
        id,
        "Upgraded to v1.66.0. grpc.Dial is deprecated in this version, so I'll switch the two call sites to grpc.NewClient and run the tests.",
        () => afterUpgrade(ctx, id),
      ),
  });
}

export function deny(ctx: Ctx, id: CardKey): void {
  const c = card(ctx, id);
  const a = waitingApproval(ctx, id);
  if (!a || !c) return;
  a.st = "denied";
  syncProjectApproval(ctx, id, "denied");
  addAct(ctx, id, {
    kind: "approval",
    text: `You denied: ${a.cmd}`,
    result: "Denied",
    st: "fail",
  });
  feed(ctx, {
    kind: "approval",
    text: `You denied ${a.cmd} on ${cardLabel(c)}`,
    pid: c.p,
    cardId: id,
  });
  toast(ctx, "Denied");
  streamCard(
    ctx,
    id,
    "Understood, I won't run it. I can pin grpc-go to v1.65.1 instead, which needs no go.sum changes beyond one line. Want me to try that?",
    () => setState(ctx, c, "needs", { reason: "Question: pin to v1.65.1 instead?" }),
  );
}

const PLAN_FIRST_FILE_MS = 2200;

export function approvePlan(ctx: Ctx, id: CardKey): void {
  const c = card(ctx, id);
  const p = waitingPlan(ctx, id);
  if (!p || !c) return;
  p.st = "approved";
  p.editing = false;
  addAct(ctx, id, { kind: "approval", text: "You approved the plan", result: "Approved" });
  feed(ctx, {
    kind: "plan",
    text: `You approved the plan on ${cardLabel(c)}`,
    pid: c.p,
    cardId: id,
  });
  if (c.perm === "Plan only") c.perm = "Auto-accept edits";
  const first = p.steps[0] ?? "";
  c.doing = first;
  setState(ctx, c, "working");
  toast(ctx, "Plan approved");
  const step = first.charAt(0).toLowerCase() + first.slice(1);
  streamCard(ctx, id, `Thanks. Starting with step 1: ${step}.`, () => {
    runTool(ctx, id, {
      icon: "file-plus",
      action: `Created ${p.files[0] || "new file"}`,
      result: "+84 −0",
      ms: PLAN_FIRST_FILE_MS,
      done: () => {
        c.doing = p.steps[1] || c.doing;
      },
    });
  });
}

export function rejectPlan(ctx: Ctx, id: CardKey): void {
  const c = card(ctx, id);
  const p = waitingPlan(ctx, id);
  if (!p || !c) return;
  p.st = "rejected";
  p.editing = false;
  addAct(ctx, id, {
    kind: "approval",
    text: "You rejected the plan",
    result: "Rejected",
    st: "fail",
  });
  c.doing = "Reworking the plan";
  setState(ctx, c, "planning");
  toast(ctx, "Plan rejected");
  streamCard(
    ctx,
    id,
    "Understood. I'll rework the plan. Tell me what should change, or I'll propose a smaller first step.",
  );
}

export function editPlan(ctx: Ctx, id: CardKey, on: boolean): void {
  const p = waitingPlan(ctx, id);
  if (p) p.editing = on;
}

/** Saves edited plan steps, one per line; blank lines are dropped. */
export function savePlan(ctx: Ctx, id: CardKey, text: string): void {
  const p = waitingPlan(ctx, id);
  if (!p) return;
  p.steps = text
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
  p.editing = false;
  p.edited = true;
  addAct(ctx, id, { kind: "tool", text: "You edited the plan", result: `${p.steps.length} steps` });
  toast(ctx, "Plan saved");
}

export function toggleTool(ctx: Ctx, id: CardKey, msgId: string): void {
  const x = (ctx.S.chat[id] ?? []).find((z) => z.id === msgId);
  if (x?.k === "tool") x.open = !x.open;
}
