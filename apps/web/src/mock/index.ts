/**
 * The store the screens read. Views read `M.S` (a Solid mutable store, so reads are tracked) and
 * call the actions on `M`. What is still mock is simulated here; what the daemon fills (the
 * connection, the projects, and the agent catalog) is mirrored in by `~/sync`. The same object
 * is on `window.M`, where the browser console and end-to-end tests can drive it.
 */

import { createData } from "~/data";
import type { KeyValueStore } from "~/data/storage";
import { attachToPage } from "./attach";
import { applyTheme } from "./dom/theme";
import { createMarshal, type Marshal } from "./marshal";

function browserStorage(): KeyValueStore | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

function boot(): Marshal {
  const storage = browserStorage();
  return attachToPage(
    createMarshal({
      hash: window.location.hash,
      storage,
      viewport: { w: window.innerWidth, h: window.innerHeight },
      applyTheme,
      // Vite forwards `/v1` to the dev daemon, so the address is the page's own. Only the dev build
      // asks the dev server for the daemon's token.
      data: createData({ baseUrl: "", storage, dev: import.meta.env.DEV }),
    }),
  );
}

/* Reuse an existing instance, like the prototype's `if (window.M) return`, so a hot
   reload never starts a second connection or simulation. */
export const M: Marshal = window.M ?? boot();
export const S = M.S;

export type { CardView } from "./deco";
export type { MsgView } from "./deco-msgs";
export type { Marshal } from "./marshal";
export type * from "./settings-types";
export type { State } from "./state-types";
export type * from "./types";
