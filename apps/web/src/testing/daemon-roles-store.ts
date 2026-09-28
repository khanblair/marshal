/**
 * The store a component test follows when the roles (section S27) are on the daemon. It works the
 * way `daemon-providers-store.ts` does: a test file that imports this module before anything that
 * reads `~/mock` gives the screens a store that follows a fake daemon, so `M.saveRole`, `M.createRole`,
 * `M.duplicateRole`, `M.resetRole`, `M.deleteRole`, and `M.importRoles` take the daemon's path.
 *
 * Every settings section the shared test store keeps on the mock is pinned again here, and the roles
 * are the one taken back off it, so the only thing that comes from this daemon is the roles, plus the
 * projects and the agent catalog the daemon always holds. It answers with the golden `role-list`:
 * Worker and Reviewer, which Marshal ships, and Nightly janitor, which a person made.
 */
import { afterAll, beforeAll, expect, vi } from "vitest";
import { applyRoleList } from "~/sync/roles";
import { createFakeDaemon, type FakeDaemon } from "./fake-daemon";
import { createRolesStore } from "./fake-roles";
import { PROTOTYPE_PROJECTS } from "./projects";
import {
  contextOf,
  createTestMarshal,
  MOCK_HISTORY,
  MOCK_PERSON_SECTIONS,
  MOCK_SETTINGS_SECTIONS,
} from "./test-store";

/** The daemon the component tests of this file drive. */
export const daemon: FakeDaemon = createFakeDaemon({ projects: PROTOTYPE_PROJECTS });

const M = createTestMarshal({
  data: daemon.data,
  sections: {
    ...MOCK_HISTORY,
    ...MOCK_PERSON_SECTIONS,
    ...MOCK_SETTINGS_SECTIONS,
    S27: "daemon",
    S17: "mock",
    S20: "mock",
    S7c: "mock",
    S9: "mock",
  },
});
window.M = M;

/** The store's context, for a test that reads or fills what the syncers hold. */
const ctx = contextOf(M);

/** Puts the daemon and the store back to the golden list, so one test cannot see another's change. */
export function resetRoles(): void {
  Object.assign(daemon.roles, createRolesStore());
  daemon.calls.length = 0;
  applyRoleList(ctx, { roles: daemon.roles.rows, serverTime: daemon.roles.now() });
  M.set({ roleSel: "Worker", toasts: [], dialog: null });
}

beforeAll(async () => {
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
});

afterAll(() => daemon.data.stop());
