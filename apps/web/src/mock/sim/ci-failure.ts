import { type CardKey, cardLabel } from "../card-key";
import type { Ctx } from "../context";
import { addAct, feed, notice, pushMsg, runTool, seq, setState, streamCard } from "../engine";
import { card } from "../selectors";
import type { Card } from "../types";

const DEFAULT_CARD_ID: CardKey = "api#40";
/** Pull request number given to a card that fails CI before it had one. */
const FALLBACK_PR = 290;
/** "Simulate CI failure" plays the same story faster. */
const FAST_SPEED = 0.35;
/** Delay of each step after the previous one, at normal speed. */
const STEP_MS = {
  firstFailure: 20_000,
  firstFailureFast: 3000,
  rerun: 3000,
  rerunFailed: 5000,
  agentReply: 1200,
  fixEdit: 4000,
  push: 1500,
  gaveUp: 6000,
};

const failingTest = (c: Card): string => {
  if (c.p === "api") return "TestRetryBackoff";
  return c.p === "web" ? "Reports export test" : "LoginFlowTest";
};

function rerunFailed(ctx: Ctx, c: Card, test: string): void {
  c.ci = "failed";
  const latest = ctx.S.act[c.id]?.[0];
  if (latest) {
    latest.st = "fail";
    latest.result = "Failed again";
  }
  pushMsg(
    ctx,
    c.id,
    ctx.msg.system("The rerun failed too. Sent the trimmed log of the failed step to the agent."),
  );
  c.doing = `Fixing ${test} after CI failure`;
  setState(ctx, c, "working");
}

function gaveUp(ctx: Ctx, c: Card, test: string): void {
  c.ci = "failed";
  addAct(ctx, c.id, {
    kind: "test",
    text: `CI failed: test job on ${c.branch}`,
    result: `${test} failed, attempt 3`,
    st: "fail",
  });
  pushMsg(
    ctx,
    c.id,
    ctx.msg.system(
      "CI failed 3 times. The fix loop reached its round limit, so the card needs you.",
    ),
  );
  setState(ctx, c, "needs", { reason: `CI failed 3 times: ${test}` });
  const tests = ctx.S.checks[c.id]?.[0];
  if (tests) tests.st = "failed";
  feed(ctx, {
    kind: "ci",
    text: `CI failed 3 times on ${cardLabel(c)} ${c.title}`,
    pid: c.p,
    cardId: c.id,
  });
  notice(ctx, {
    kind: "ci",
    cardId: c.id,
    text: `CI failed on ${cardLabel(c)}`,
    sub: `${c.title}. ${test} failed 3 times.`,
  });
  ctx.flags.ciFailRunning = false;
}

function fixSteps(ctx: Ctx, c: Card, k: number) {
  return [
    {
      afterMs: STEP_MS.agentReply * k,
      run: () =>
        streamCard(
          ctx,
          c.id,
          "The test expects the third retry within 400 ms, but jitter can push it to 450 ms. I'll cap the jitter at 25 percent of the base delay.",
        ),
    },
    {
      afterMs: STEP_MS.fixEdit * k,
      run: () =>
        runTool(ctx, c.id, {
          icon: "file-pen",
          action: "Edited internal/proxy/retry.go",
          result: "+3 −1",
          ms: 900,
        }),
    },
    {
      afterMs: STEP_MS.push * k,
      run: () =>
        runTool(ctx, c.id, {
          icon: "upload",
          action: `Ran git push origin ${c.branch}`,
          result: "Pushed",
          ms: 900,
          kind: "command",
          done: () => {
            c.ci = "running";
            c.doing = "";
            setState(ctx, c, "review");
          },
        }),
    },
  ];
}

/**
 * CI fails on a card in review, a rerun fails too, the agent tries a fix, and after
 * the third failure the card needs you. Only one runs at a time.
 */
export function ciFailure(ctx: Ctx, id: CardKey | undefined, fast: boolean): void {
  const c = card(ctx, id || DEFAULT_CARD_ID);
  if (!c || ctx.flags.ciFailRunning) return;
  ctx.flags.ciFailRunning = true;
  const k = fast ? FAST_SPEED : 1;
  if (c.state !== "review") {
    c.pr = c.pr || FALLBACK_PR;
    setState(ctx, c, "review");
  }
  c.ci = "running";
  const test = failingTest(c);
  seq([
    {
      afterMs: fast ? STEP_MS.firstFailureFast : STEP_MS.firstFailure,
      run: () => {
        c.ci = "failed";
        addAct(ctx, c.id, {
          kind: "test",
          text: `CI failed: test job on ${c.branch}`,
          result: `${test} failed`,
          st: "fail",
        });
        pushMsg(
          ctx,
          c.id,
          ctx.msg.system(
            "CI failed on the test job. Marshal is rerunning it once in case the test is flaky.",
          ),
        );
      },
    },
    {
      afterMs: STEP_MS.rerun * k,
      run: () => {
        c.ci = "running";
        addAct(ctx, c.id, {
          kind: "test",
          text: "Reran the test job",
          result: "Running",
          st: "running",
        });
      },
    },
    { afterMs: STEP_MS.rerunFailed * k, run: () => rerunFailed(ctx, c, test) },
    ...fixSteps(ctx, c, k),
    { afterMs: STEP_MS.gaveUp * k, run: () => gaveUp(ctx, c, test) },
  ]);
}
