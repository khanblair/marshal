/**
 * The store a component test follows when the person's sections (the profile S2a, the saved views
 * S6a, and the screen preferences S32) are on the daemon. It works the way `daemon-cards-store.ts`
 * does: a test file that imports this module before anything that reads `~/mock` gives the screens
 * a store that follows a fake daemon, so `M.saveProfile` and `M.saveView` take the daemon's path.
 *
 * The daemon holds the prototype's three projects, the golden profile, and one saved view for the
 * api project. The file chooser is a function the test answers (`chooser`).
 */
import { afterAll, beforeAll, expect, vi } from "vitest";
import { createFakeDaemon, type FakeDaemon } from "./fake-daemon";
import { wireSavedView } from "./fake-saved-views";
import { PROTOTYPE_PROJECTS } from "./projects";
import { contextOf, createTestMarshal, DAEMON_PERSON } from "./test-store";

/** The daemon the component tests of this file drive. */
export const daemon: FakeDaemon = createFakeDaemon({
  projects: PROTOTYPE_PROJECTS,
  savedViews: [
    wireSavedView({
      id: "01M3C107JB041061050R3GG2V1",
      projectId: "api",
      name: "Needs me",
      filters: [{ key: "status", value: "needs" }],
    }),
  ],
});

/** What the file chooser answers: a file, or null for a chooser closed without one. */
export const chooser = { pick: vi.fn<() => Promise<File | null>>() };

const M = createTestMarshal({
  data: daemon.data,
  sections: DAEMON_PERSON,
  pickImage: () => chooser.pick(),
});
window.M = M;

/** The store's context, for a test that reads what the syncers hold. */
export const ctx = contextOf(M);

beforeAll(async () => {
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
});

afterAll(() => daemon.data.stop());
