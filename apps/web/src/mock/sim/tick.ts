import { batch } from "solid-js";
import type { Ctx } from "../context";
import { addAct, announce, feed } from "../engine";
import { card } from "../selectors";
import type { Card, SleepNotice } from "../types";

/** What each busy card cycles through while it works. */
const DOING: Record<number, readonly string[]> = {
  42: [
    "Running k6 at 500 requests per second",
    "Reading p99 latency from the k6 summary",
    "Raising load to 1,000 requests per second",
  ],
  118: [
    "Updating theme tokens in settings.tsx",
    "Running pnpm test settings",
    "Capturing screenshots in light and dark",
  ],
  209: [
    "Running ./gradlew :app:testDebugUnitTest",
    "Editing BiometricPromptManager.kt",
    "Running ./gradlew lint",
  ],
  213: [
    "Reading the failed step log from the android workflow",
    "Editing LoginFlowTest.kt",
    "Running ./gradlew connectedCheck",
  ],
};

/** Working cards spend a little every second, more for some ids than others. */
const COST_PER_TICK_USD = 0.004;
const COST_STEP_USD = 0.002;
const COST_BUCKETS = 5;
/** A busy card moves to its next activity every fourth tick, staggered by id. */
const DOING_EVERY_TICKS = 4;
/** The "Fix until e2e passes" loop on #213 reports a round every 40 ticks. */
const LOOP_ROUND_TICKS = 40;
const LOOP_ROUNDS_BEFORE_LOAD = 2;

function advance(ctx: Ctx, c: Card, n: number): void {
  c.cost += COST_PER_TICK_USD + (c.id % COST_BUCKETS) * COST_STEP_USD;
  const steps = DOING[c.id];
  if (!steps || n % DOING_EVERY_TICKS !== c.id % DOING_EVERY_TICKS) return;
  c.doing = steps[(steps.indexOf(c.doing) + 1) % steps.length] ?? c.doing;
  c.upd = Date.now();
  addAct(ctx, c.id, { kind: /Running/.test(c.doing) ? "command" : "file", text: c.doing });
}

/** When the sleep notice runs out, its idle cards go to sleep unless pinned, working, or waiting on you. */
function sleepIdleCards(ctx: Ctx): void {
  const { S } = ctx;
  const n = S.notices.find((x): x is SleepNotice => x.kind === "sleep");
  if (!n || Date.now() < n.deadline) return;
  for (const id of n.cards) {
    const c = card(ctx, id);
    if (c && !c.pinned && c.state !== "working" && c.state !== "needs") c.asleep = true;
  }
  S.notices = S.notices.filter((x) => x !== n);
  announce(ctx, `${n.cards.length} idle cards went to sleep`);
}

/** One second of the simulated daemon. */
export function tick(ctx: Ctx): void {
  batch(() => {
    ctx.flags.tickN += 1;
    const n = ctx.flags.tickN;
    if (n % LOOP_ROUND_TICKS === 0) {
      const round = n / LOOP_ROUND_TICKS + LOOP_ROUNDS_BEFORE_LOAD;
      feed(ctx, {
        kind: "schedule",
        text: `Fix until e2e passes ran round ${round} on #213`,
        pid: "mobile",
        cardId: 213,
      });
    }
    for (const c of ctx.S.cards) {
      if (c.state === "working" && !c.paused && !c.asleep) advance(ctx, c, n);
    }
    sleepIdleCards(ctx);
    ctx.clock.pulse();
  });
}
