/**
 * The store a component test follows when the notices (section S23) are on the daemon. It works the
 * way `daemon-limits-store.ts` does: a test file that imports this module before anything that reads
 * `~/mock` gives the screens a store that follows a fake daemon, so `M.keepAwake`, `M.keepAllAwake`,
 * `M.sleepAll`, and `M.dismissNotice` take the daemon's path.
 *
 * The sections the shared test store pins to the mock are pinned here too, so the only thing that
 * comes from this daemon is the notices, plus the projects and the cards the daemon always holds. The
 * daemon answers with the golden `notice-list`: one sleep group for one project, naming two cards by
 * their opaque ids. The two cards are held by the daemon under those exact ids, because a sleep
 * notice names its cards by those ids and the store works in card keys, so a notice whose cards the
 * store cannot place is not drawn at all.
 */

import type { Card as WireCard } from "@marshal/protocol";
import { afterAll, beforeAll, expect, vi } from "vitest";
import { applyNoticeList } from "~/sync/notices";
import { wireCard } from "./fake-cards";
import { createFakeDaemon, type FakeDaemon } from "./fake-daemon";
import { createNoticesStore } from "./fake-notices";
import { PROTOTYPE_PROJECTS } from "./projects";
import { contextOf, createTestMarshal, MOCK_HISTORY, MOCK_PERSON_SECTIONS } from "./test-store";

/** The two card ids the golden `notice-list` names, under the project the notice belongs to. */
const NOTICE_CARD_IDS = ["01JD7Q4M2X8K9V0P5T3RB6NHC3", "01JD7Q4M2X8K9V0P5T3RB6NHC4"];

/** The cards the daemon holds, so the sleep notice's cards can be placed. */
const NOTICE_CARDS: readonly WireCard[] = NOTICE_CARD_IDS.map((id, i) =>
  wireCard({
    id,
    projectId: "web",
    number: i + 1,
    title: i === 0 ? "Refresh the token" : "CSV export",
    state: "ready",
    session: "awake",
  }),
);

/** The daemon the component tests of this file drive. */
export const daemon: FakeDaemon = createFakeDaemon({
  projects: PROTOTYPE_PROJECTS,
  cards: NOTICE_CARDS,
});

const M = createTestMarshal({
  data: daemon.data,
  sections: {
    ...MOCK_HISTORY,
    ...MOCK_PERSON_SECTIONS,
    S17: "mock",
    S20: "mock",
    S7c: "mock",
    S9: "mock",
    S23: "daemon",
  },
});
window.M = M;

/** The store's context, for a test that reads or fills what the syncers hold. */
const ctx = contextOf(M);

/** Puts the daemon and the store back to the golden list, so one test cannot see another's change. */
export function resetNotices(): void {
  const fresh = createNoticesStore({ publish: daemon.notices.publish, now: daemon.notices.now });
  daemon.notices.notices = fresh.notices;
  daemon.calls.length = 0;
  applyNoticeList(ctx, { notices: daemon.notices.notices });
  M.set({ toasts: [], dialog: null, noticesOpen: true, openId: null });
}

beforeAll(async () => {
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
});

afterAll(() => daemon.data.stop());
