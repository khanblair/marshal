/**
 * The store a component test follows when the cost numbers (S19b) and the limits (S26b) are on the
 * daemon. It works the way `daemon-providers-store.ts` does: a test file that imports this module
 * before anything that reads `~/mock` gives the screens a store that follows a fake daemon, so
 * `M.saveLimits` and the cost and awake numbers take the daemon's path.
 *
 * The sections the shared test store pins to the mock are pinned here too, so the only thing that
 * comes from this daemon is the cost and the limits, plus the projects and the Home numbers the
 * daemon always holds. The daemon answers with the golden `limit-list`: $25 a day, $400 a month, and
 * 18 awake cards for the whole install, and a daily ceiling for a project id the store has no
 * project for (so every project the screens draw has no ceiling of its own).
 */
import { afterAll, beforeAll, expect, vi } from "vitest";
import { createFakeDaemon, type FakeDaemon } from "./fake-daemon";
import { createLimitsStore } from "./fake-limits";
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
    S19b: "daemon",
    S26b: "daemon",
  },
});
window.M = M;

/** The store's context, for a test that reads or fills what the syncers hold. */
export const ctx = contextOf(M);

/** Puts the daemon and the store back to the golden list, so one test cannot see another's save. */
export function resetLimits(): void {
  Object.assign(daemon.limits, createLimitsStore());
  daemon.calls.length = 0;
}

beforeAll(async () => {
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
});

afterAll(() => daemon.data.stop());
