import type { ApiClient } from "~/data/api-client";
import { type SleepChoice, toSleepChoice } from "~/data/mappers/sleep-settings";
import type { Ctx } from "~/mock/context";
import type { Syncer } from "./syncer";

/**
 * Section S26a: the numbers and choices behind automatic sleep. The daemon holds them (the `settings`
 * table) and the idle timer runs on them, so the store mirrors what it says rather than the other way
 * round.
 *
 * There are no sleep-setting events: the route answers the whole record, and a load and a save both
 * arrive through `applySleepChoice`. It publishes no topic, so it is loaded once when the app comes
 * online and after every re-sync, like the limits (S26b).
 */
export const sleepSettingsSyncer: Syncer<SleepChoice> = {
  section: "S26a",
  topics: [],
  async load(api: ApiClient) {
    return toSleepChoice(await api.sleepSettings());
  },
  apply(ctx, choice) {
    applySleepChoice(ctx, choice);
  },
};

/** The daemon's settings as the store and the form hold them. It is one write, so a screen redraws once. */
export function applySleepChoice(ctx: Ctx, choice: SleepChoice): void {
  ctx.S.sleep = choice;
}
