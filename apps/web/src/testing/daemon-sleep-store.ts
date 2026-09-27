/**
 * The store a component test follows when the sleep settings (section S26a) are on the daemon. It
 * works the way `daemon-limits-store.ts` does: a test file that imports this module before anything
 * that reads `~/mock` gives the screens a store that follows a fake daemon, so `M.saveSleepSettings`
 * takes the daemon's path and the Sessions panel of Settings reads and writes the daemon.
 *
 * The sections the shared test store pins to the mock are pinned here too, so the only thing that
 * comes from this daemon is the sleep settings, plus the projects the daemon always holds. The
 * daemon answers with the golden `sleep-settings`: idle after 15 minutes, a 2-minute warning, 15
 * minutes of Keep awake, automatic restore, and warnings in the app.
 */
import { afterAll, beforeAll, expect, vi } from "vitest";
import { toSleepChoice } from "~/data/mappers/sleep-settings";
import { applySleepChoice } from "~/sync/sleep-settings";
import { createFakeDaemon, type FakeDaemon } from "./fake-daemon";
import { createSleepStore } from "./fake-sleep";
import { PROTOTYPE_PROJECTS } from "./projects";
import { contextOf, createTestMarshal, MOCK_HISTORY, MOCK_PERSON_SECTIONS } from "./test-store";

/** The daemon the component tests of this file drive. */
export const daemon: FakeDaemon = createFakeDaemon({ projects: PROTOTYPE_PROJECTS });

const M = createTestMarshal({
  data: daemon.data,
  sections: {
    ...MOCK_HISTORY,
    ...MOCK_PERSON_SECTIONS,
    S17: "mock",
    S20: "mock",
    S7c: "mock",
    S9: "mock",
    S26a: "daemon",
  },
});
window.M = M;

/** The store's context, for a test that reads or fills what the syncer holds. */
export const ctx = contextOf(M);

/** Puts the daemon and the store back to the golden record, so one test cannot see another's save. */
export function resetSleep(): void {
  daemon.sleep.settings = createSleepStore().settings;
  daemon.calls.length = 0;
  applySleepChoice(ctx, toSleepChoice(daemon.sleep.settings));
  M.set({ toasts: [], dialog: null });
}

beforeAll(async () => {
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
});

afterAll(() => daemon.data.stop());
