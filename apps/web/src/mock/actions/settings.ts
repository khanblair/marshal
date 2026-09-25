import type { Ctx } from "../context";
import { addAct, later, toast } from "../engine";
import { card } from "../selectors";
import { ciFailure } from "../sim/ci-failure";
import type { Theme } from "../types";

const DEFAULT_CI_CARD_ID = 40;

export function simulateCiFailure(ctx: Ctx, id?: number): void {
  const target = id || DEFAULT_CI_CARD_ID;
  ciFailure(ctx, target, true);
  toast(ctx, `Simulating a CI failure on #${target}`);
}

export function setTheme(ctx: Ctx, theme: Theme): void {
  ctx.S.theme = theme;
  ctx.env.applyTheme(ctx.S);
}

/** Checks finish one after another; the first fails while the card's CI is failing. */
const FIRST_CHECK_MS = 1500;
const NEXT_CHECK_MS = 900;

export function runChecks(ctx: Ctx, id: number): void {
  const ks = ctx.S.checks[id];
  if (!ks) return;
  for (const k of ks) if (k.cmd) k.st = "running";
  ks.forEach((k, i) => {
    if (!k.cmd) return;
    later(FIRST_CHECK_MS + i * NEXT_CHECK_MS, () => {
      k.st = card(ctx, id)?.ci === "failed" && i === 0 ? "failed" : "passed";
      const passed = k.st === "passed";
      addAct(ctx, id, {
        kind: "test",
        text: `Check: ${k.name}`,
        result: passed ? "Passed" : "Failed",
        st: passed ? "ok" : "fail",
      });
    });
  });
}
