/**
 * The fake Marshal daemon, ported from design/store.js. Views read `M.S` (a Solid
 * mutable store, so reads are tracked) and call the actions on `M`. The same object is
 * on `window.M`, where the parity harness drives it.
 */
import { applyTheme, watchSystemTheme } from "./dom/theme";
import { watchResize } from "./dom/viewport";
import { createMarshal, type Marshal } from "./marshal";
import type { KeyValueStore } from "./storage";

declare global {
  interface Window {
    M?: Marshal;
  }
}

function browserStorage(): KeyValueStore | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

function boot(): Marshal {
  const M = createMarshal({
    hash: window.location.hash,
    storage: browserStorage(),
    viewport: { w: window.innerWidth, h: window.innerHeight },
    applyTheme,
  });
  window.M = M;
  applyTheme(M.S);
  watchSystemTheme(M.S);
  watchResize(M);
  M.S.ready = true;
  window.dispatchEvent(new Event("marshal-ready"));
  return M;
}

/* Reuse an existing instance, like the prototype's `if (window.M) return`, so a hot
   reload never starts a second simulation. */
export const M: Marshal = window.M ?? boot();
export const S = M.S;

export type { CardView } from "./deco";
export type { MsgView } from "./deco-msgs";
export type { Marshal } from "./marshal";
export type * from "./settings-types";
export type { State } from "./state-types";
export type * from "./types";
