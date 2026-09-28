/**
 * The store a component test follows when the provider keys (section S28) are on the daemon. It
 * works the way `daemon-person-store.ts` does: a test file that imports this module before anything
 * that reads `~/mock` gives the screens a store that follows a fake daemon, so `M.saveProviderKey`
 * and `M.testProviderKey` take the daemon's path.
 *
 * The sections the shared test store pins to the mock are pinned here too, so the only thing that
 * comes from this daemon is the provider keys. The daemon holds the prototype's projects and the
 * golden provider list: Anthropic saved, DeepSeek empty, OpenRouter invalid, Ollama saved with its
 * address.
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
