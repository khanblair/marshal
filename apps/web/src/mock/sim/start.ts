import { isDaemon } from "~/data/sections";
import type { CardKey } from "../card-key";
import { type Ctx, sectionsOf } from "../context";
import { later } from "../engine";
import { ciFailure } from "./ci-failure";
import { script41, script46, scriptMerge35 } from "./scripts";
import { tick } from "./tick";

const SCRIPT41_START_MS = 1500;
const SCRIPT46_START_MS = 4000;
const MERGE35_START_MS = 3000;
const CI_FAILURE_START_MS = 5000;
const TICK_MS = 1000;
const CI_FAILURE_CARD_ID: CardKey = "api#40";

/**
 * Starts the fake daemon. Hash flags switch parts off: `nosim` everything, `n41` and
 * `n46` the scripted cards, `nm` the #35 merge, `nci` the CI failure on #40, `nt` the tick.
 */
export function startSimulation(ctx: Ctx, hash: string): void {
  if (/nosim/.test(hash)) return;
  // The scripted cards, the merge, the CI failure, and the tick all write to the mock's own cards.
  // Once S5a is on the daemon those cards are the daemon's, and a simulation writing to them would
  // be the app making up state the daemon owns, so nothing is started at all.
  if (isDaemon("S5a", sectionsOf(ctx.env))) return;
  if (!/n41/.test(hash)) later(SCRIPT41_START_MS, () => script41(ctx));
  if (!/n46/.test(hash)) later(SCRIPT46_START_MS, () => script46(ctx));
  if (!/nm/.test(hash)) later(MERGE35_START_MS, () => scriptMerge35(ctx));
  if (!/nci/.test(hash))
    later(CI_FAILURE_START_MS, () => ciFailure(ctx, CI_FAILURE_CARD_ID, false));
  if (!/nt/.test(hash)) setInterval(() => tick(ctx), TICK_MS);
}
