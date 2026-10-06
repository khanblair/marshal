/**
 * The store a component test follows when the schedules (S30) are the daemon's. It works the way
 * `daemon-integrations-store.ts` does: a test file that imports this module before anything that reads
 * `~/mock` gives the screens a store that follows a fake daemon, so `M.createSchedule`,
 * `M.saveSchedule`, `M.runSchedule`, and `M.scheduleCatalog` take the daemon's path. The daemon holds
 * the golden schedule list and {@link FAKE_CATALOG}, and the connections it has none of are not set up.
 */
import { afterAll, beforeAll, expect, vi } from "vitest";
import { createFakeDaemon, type FakeDaemon } from "./fake-daemon";
import { PROTOTYPE_PROJECTS } from "./projects";
import { createTestMarshal, MOCK_HISTORY, MOCK_PERSON_SECTIONS } from "./test-store";

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
  },
});
window.M = M;

beforeAll(async () => {
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
});

afterAll(() => daemon.data.stop());
