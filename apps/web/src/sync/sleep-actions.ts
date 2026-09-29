import type { SleepChoice } from "~/data/mappers/sleep-settings";
import { toSleepChoice, toWireSleepSettings } from "~/data/mappers/sleep-settings";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";
import { applySleepChoice } from "./sleep-settings";

/**
 * Saves the whole sleep record (section S26a). The screen's three `Select`s each change one field
 * and the route takes the record as one write, so the form's own state is drawn first and put back
 * if the daemon says no. A refusal is shown in the daemon's own words by `ctx.optimistic` - "Choose
 * an idle time of 5, 15, 30, or 60 minutes." is a sentence the form shows too - and the daemon's
 * answer is what the store keeps, so a value the daemon normalised is not left on screen as typed.
 */
export async function saveSleepSettings(ctx: Ctx, next: SleepChoice): Promise<boolean> {
  const api = ctx.env.data?.api;
  const before = ctx.S.sleep;
  if (!api) return false;
  try {
    const saved = await ctx.optimistic({
      key: "sleep-settings",
      apply: () => {
        ctx.S.sleep = next;
      },
      request: () => api.saveSleepSettings(toWireSleepSettings(next)),
      rollback: () => {
        ctx.S.sleep = before;
      },
    });
    applySleepChoice(ctx, toSleepChoice(saved));
    toast(ctx, "Saved");
    return true;
  } catch {
    return false;
  }
}
