import type { State } from "../state-types";

interface ResizeTarget {
  /** True while the shell draws the app inside a device frame and sets the size itself. */
  readonly _framed: boolean;
  S: State;
  set(patch: Partial<State>): void;
}

/** Follows the window width, unless the app is drawn inside a device frame. */
export function watchResize(m: ResizeTarget): void {
  window.addEventListener("resize", () => {
    if (!m._framed && m.S.vw !== window.innerWidth) m.set({ vw: window.innerWidth });
  });
}
