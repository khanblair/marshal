import { batch } from "solid-js";
import type { Ctx } from "../context";
import {
  addAct,
  feed,
  later,
  live,
  notice,
  pushMsg,
  runTool,
  seq,
  setState,
  streamCard,
} from "../engine";
import { takeMid } from "../ids";
import { card } from "../selectors";
import type { Activity, Card, ChecklistItem } from "../types";

const TOKEN_REFRESH_CARD_ID = 41;
const CONFIG_SPLIT_CARD_ID = 46;
const MERGING_CARD_ID = 35;
/** Pull request #41 opens at the end of its script. */
const TOKEN_REFRESH_PR = 287;
/** The race-detector checklist item is ticked just after the race run finishes. */
const RACE_ITEM_DONE_MS = 6200;
const MERGE_STEP_MS = 2200;
const MERGE_STEP_PCT = 9;
const MERGE_DONE_PCT = 100;

/** #41 finishes its test run, runs the race detector, and opens pull request #287. */
export function script41(ctx: Ctx): void {
  const id = TOKEN_REFRESH_CARD_ID;
  const { S } = ctx;
  const c = card(ctx, id);
  const last = S.chat[id]?.at(-1);
  if (!c || last?.k !== "tool") return;
  const act0 = live<Activity>({
    id: `a${takeMid(ctx.ids)}`,
    kind: "test",
    text: "Ran go test ./internal/auth/...",
    result: "Running",
    st: "running",
    ts: Date.now(),
  });
  S.act[id]?.unshift(act0);
  seq([
    {
      afterMs: 3500,
      run: () => {
        last.st = "ok";
        last.result = "14 passed";
        act0.st = "ok";
        act0.result = "14 passed";
        c.doing = "Writing a summary of the test run";
      },
    },
    {
      afterMs: 600,
      run: () =>
        streamCard(
          ctx,
          id,
          "All 14 auth tests pass, including the new test for concurrent refresh. This change touches shared state, so I'll run the full suite with the race detector next.",
        ),
    },
    { afterMs: 4200, run: () => raceDetector(ctx, c) },
    {
      afterMs: 6800,
      run: () =>
        streamCard(
          ctx,
          id,
          "No races found. I'll commit, push the branch, and open a pull request.",
        ),
    },
    ...shipSteps(ctx, c),
  ]);
}

function raceDetector(ctx: Ctx, c: Card): void {
  const item: ChecklistItem | undefined = c.checklists[0]?.items[3];
  c.doing = "Running go test -race ./...";
  later(RACE_ITEM_DONE_MS, () => {
    if (!item || item.done) return;
    item.done = true;
    item.by = "agent";
    item.doneAt = Date.now();
    addAct(ctx, c.id, { kind: "tool", text: `${c.agent} completed ${item.text}` });
  });
  runTool(ctx, c.id, {
    icon: "terminal",
    action: "Ran go test -race ./...",
    result: "231 passed, no races",
    ms: 6000,
    kind: "test",
  });
}

function shipSteps(ctx: Ctx, c: Card) {
  const id = c.id;
  return [
    {
      afterMs: 2600,
      run: () => {
        c.doing = "Pushing marshal/41-fix-token-refresh";
        runTool(ctx, id, {
          icon: "git-commit-horizontal",
          action: 'Committed "Share one token refresh between concurrent requests"',
          result: "1 commit",
          ms: 900,
        });
      },
    },
    {
      afterMs: 1200,
      run: () =>
        runTool(ctx, id, {
          icon: "upload",
          action: "Ran git push origin marshal/41-fix-token-refresh",
          result: "Pushed",
          ms: 1400,
          kind: "command",
        }),
    },
    {
      afterMs: 1800,
      run: () =>
        runTool(ctx, id, {
          icon: "git-pull-request",
          action: "Opened pull request #287",
          result: "Checks started",
          ms: 800,
          kind: "tool",
          done: () => {
            c.pr = TOKEN_REFRESH_PR;
            c.ci = "running";
            c.doing = "";
            setState(ctx, c, "review");
            streamCard(
              ctx,
              id,
              "Pull request #287 is open. The Reviewer is reading it now, and CI is running.",
            );
          },
        }),
    },
    { afterMs: 9000, run: () => ciPassed(ctx, c) },
  ];
}

function ciPassed(ctx: Ctx, c: Card): void {
  c.ci = "passed";
  addAct(ctx, c.id, {
    kind: "test",
    text: "CI passed on marshal/41-fix-token-refresh",
    result: "test, lint",
  });
  feed(ctx, {
    kind: "ci",
    text: "CI passed on marshal/41-fix-token-refresh",
    pid: "api",
    cardId: TOKEN_REFRESH_CARD_ID,
  });
  for (const k of ctx.S.checks[c.id] ?? []) if (k.id !== "k3") k.st = "passed";
}

/** #46 maps the callers of config.Load and posts a plan for review. */
export function script46(ctx: Ctx): void {
  const id = CONFIG_SPLIT_CARD_ID;
  const c = card(ctx, id);
  if (!c) return;
  seq([
    {
      afterMs: 5000,
      run: () => {
        c.doing = "Mapping 38 callers of config.Load";
        runTool(ctx, id, {
          icon: "search",
          action: "Searched codebase map for config.Load",
          result: "38 callers in 11 packages",
          ms: 1500,
          kind: "tool",
        });
      },
    },
    {
      afterMs: 4500,
      run: () =>
        streamCard(
          ctx,
          id,
          "Here is my plan. Nothing changes for callers until the last step.",
          () => postPlan46(ctx, c),
        ),
    },
  ]);
}

function postPlan46(ctx: Ctx, c: Card): void {
  const id = c.id;
  pushMsg(
    ctx,
    id,
    ctx.msg.plan({
      st: "waiting",
      editing: false,
      steps: [
        "Move env parsing into internal/config/env with no API change",
        "Move file loading into internal/config/file",
        "Keep config.Load as a thin wrapper over both",
        "Update the 38 callers only where they reach into private fields",
        "Delete the old helpers once tests pass",
      ],
      files: [
        "internal/config/config.go",
        "internal/config/env/env.go",
        "internal/config/file/file.go",
        "11 caller packages",
      ],
      risks: [
        "#39 also edits internal/config/logging.go. I claimed that file and will rebase after #39 merges",
      ],
      checks: ["go test ./...", "go vet ./...", "No change to config.Load signature"],
    }),
  );
  c.doing = "";
  setState(ctx, c, "needs", { reason: "Plan ready for review" });
  addAct(ctx, id, {
    kind: "tool",
    text: "Posted a plan for review",
    result: "Waiting",
    st: "waiting",
  });
  feed(ctx, {
    kind: "plan",
    text: "Plan ready for review on #46",
    pid: "api",
    cardId: CONFIG_SPLIT_CARD_ID,
  });
  notice(ctx, {
    kind: "plan",
    cardId: id,
    text: "Plan ready for review on #46",
    sub: "Split config loader into packages",
  });
}

/** #35 finishes its merge a step at a time, then moves to Done. */
export function scriptMerge35(ctx: Ctx): void {
  const c = card(ctx, MERGING_CARD_ID);
  if (!c) return;
  const timer = setInterval(() => {
    c.mergePct = Math.min(MERGE_DONE_PCT, c.mergePct + MERGE_STEP_PCT);
    if (c.mergePct < MERGE_DONE_PCT) return;
    clearInterval(timer);
    batch(() => mergeDone(ctx, c));
  }, MERGE_STEP_MS);
}

function mergeDone(ctx: Ctx, c: Card): void {
  c.doing = "";
  setState(ctx, c, "done");
  addAct(ctx, c.id, { kind: "tool", text: "Merged into main", result: "Affected tests passed" });
  pushMsg(ctx, c.id, ctx.msg.system("Merged into main. Affected tests passed. Worktree removed."));
  feed(ctx, {
    kind: "merge",
    text: "#35 Health check returns build info merged into main",
    pid: "api",
    cardId: MERGING_CARD_ID,
  });
}
