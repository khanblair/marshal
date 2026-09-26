import { attachToPage } from "~/mock/attach";
import { applyTheme } from "~/mock/dom/theme";
import type { Marshal } from "~/mock/marshal";
import { createTestMarshal, MOCK_HISTORY, MOCK_PERSON_SECTIONS } from "./test-store";

/** The shared store of a test file: like the browser's, with the prototype's projects and no daemon. */
function createSharedStore(): Marshal {
  // The shared store is a mock store: it has the prototype's projects, no daemon, and the mock's own
  // chat and activity, whatever the register says. A test that wants the daemon's history builds its
  // own store and installs it, the way `daemon-cards-store.ts` does.
  //
  // S20 is pinned to mock here regardless of the register: once it is switched, the reservoir
  // (sync/reservoir.ts) stops handing `S.feed` the mock's own activity, on the assumption a live
  // daemon syncer fills it instead — which this store, having no daemon, never does. The person
  // (S2a, S6a, and S32) is pinned for the same reason: the prototype's people, its seeded saved
  // views, and its profile are the mock's own, and no daemon fills them in their place.
  //
  // S17 (the project chats) is pinned for the reservoir's sake too: once it is switched, the
  // reservoir stops handing `S.chats` the mock's own chats, and this store has no daemon to fill
  // them. S7c (pause, sleep, wake, and pin) and S9 (the terminal view's switch) are pinned because
  // their actions ask the daemon: with no daemon they can only say "not connected", and the tests of
  // the views that press them exercise the mock's own. The daemon's path is tested against
  // `daemon-cards-store.ts`.
  const M = createTestMarshal({
    storage: window.localStorage,
    applyTheme,
    sections: {
      ...MOCK_HISTORY,
      ...MOCK_PERSON_SECTIONS,
      S17: "mock",
      S20: "mock",
      S7c: "mock",
      S9: "mock",
    },
  });
  M.S.ready = true;
  return attachToPage(M);
}

/**
 * Makes `window.M` build the shared test store the first time it is read, which is when a test
 * file first imports `~/mock`. It is lazy so a test that sets the location hash or fake timers
 * first (as many do) gets a store made after that, exactly as the browser boot did, and a test
 * that never touches the store pays nothing. The app's own boot (`mock/index.ts`, which builds a
 * real connection) is not used in unit tests: they run against no daemon.
 */
export function installTestStore(): void {
  if (typeof window === "undefined") return;
  let store: Marshal | undefined;
  Object.defineProperty(window, "M", {
    configurable: true,
    get: () => {
      store ??= createSharedStore();
      return store;
    },
    set: (value: Marshal | undefined) => {
      store = value;
    },
  });
}
