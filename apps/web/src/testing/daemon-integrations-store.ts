/**
 * The store a component test follows when a connection (section S29a, today GitHub) is on the
 * daemon. It works the way `daemon-providers-store.ts` does: a test file that imports this module
 * before anything that reads `~/mock` gives the screens a store that follows a fake daemon, so
 * `M.connectGitHub`, `M.testIntegration`, and `M.disconnectIntegration` take the daemon's path and
 * `M.connectionOnDaemon("github")` is true.
 *
 * The sections the shared test store pins to the mock are pinned here too, so the only thing that
 * comes from this daemon is the connection rows. S29a is left to the register, which says the daemon
 * owns it. The daemon holds the prototype's projects and a connection list with nothing set up.
 */
import { afterAll, beforeAll, expect, vi } from "vitest";
import { createFakeDaemon, type FakeDaemon } from "./fake-daemon";
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
  },
});
window.M = M;

/** The store's context, for a test that reads what the syncers hold. */
export const ctx = contextOf(M);

beforeAll(async () => {
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
});

afterAll(() => daemon.data.stop());
