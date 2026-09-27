import type { SleepChoice } from "~/data/mappers/sleep-settings";
import { M } from "~/mock";

/**
 * One change to the sleep settings (section S26a), from the Sessions panel of Settings. The panel's
 * three `Select`s each know one field, and the daemon takes the whole record as one write, so the
 * change is drawn first and saved after.
 *
 * While S26a is still mock the store is the whole truth and the change is only a write to it. Once
 * the section is on the daemon the write is the daemon's (`sync/sleep-actions.ts`), which says
 * "Saved" itself, shows a refusal in the daemon's own words, and puts the store back if it says no.
 */
export function setSleepChoice(patch: Partial<SleepChoice>): void {
  const next = { ...M.S.sleep, ...patch };
  if (M.sleepOnDaemon()) {
    void M.saveSleepSettings(next);
    return;
  }
  M.S.sleep = next;
  M.toast("Saved");
}
