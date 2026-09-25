import { attachToPage } from "~/mock/attach";
import { applyTheme } from "~/mock/dom/theme";
import type { Marshal } from "~/mock/marshal";
import { createTestMarshal } from "./test-store";

/** The shared store of a test file: like the browser's, with the prototype's projects and no daemon. */
function createSharedStore(): Marshal {
  const M = createTestMarshal({ storage: window.localStorage, applyTheme });
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
