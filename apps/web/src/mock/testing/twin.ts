/**
 * Drives the prototype store and the port side by side on one fake clock, so a test
 * can run the same calls on both and compare every field of `S` afterwards.
 */
import { expect, vi } from "vitest";
import { applyTheme } from "../dom/theme";
import { createMarshal, type Marshal } from "../marshal";
import { loadPrototype, type ProtoM, snapshot } from "./prototype";

export const FIXED_TIME = new Date("2026-09-24T10:00:00");

/**
 * Differential tests snapshot two whole stores hundreds of times. Under coverage and a full
 * parallel run they need more than Vitest's default 5 s.
 */
export const SLOW_TEST_MS = 60_000;

export interface Twin {
  proto: ProtoM;
  port: Marshal;
  /** Calls `M[name](...args)` on both stores. */
  run(name: string, ...args: unknown[]): void;
  /** Calls a query on both stores and returns both results as plain JSON. */
  ask(name: string, ...args: unknown[]): { proto: unknown; port: unknown };
  /** Asserts both states are equal. */
  same(): void;
  /** Advances the clock `ms` in steps of `step`, asserting equal states after each step. */
  play(ms: number, step?: number): void;
  /** Runs the confirm dialog's action on both stores. */
  confirmDialog(): void;
}

function callPort(port: Marshal, name: string, args: unknown[]): unknown {
  const fn: unknown = Reflect.get(port, name);
  if (typeof fn !== "function") throw new Error(`port M.${name} is not a function`);
  return fn(...args);
}

function dialogRun(S: unknown): () => void {
  const dialog: unknown = Reflect.get(S as object, "dialog");
  const run: unknown = dialog && Reflect.get(dialog as object, "run");
  if (typeof run !== "function") throw new Error("no dialog is open");
  return () => run();
}

/** Call after `vi.useFakeTimers({ now: FIXED_TIME })`. */
export function makeTwin(hash = "#nosim"): Twin {
  window.localStorage.clear();
  const proto = loadPrototype(hash);
  const port = createMarshal({
    hash,
    storage: window.localStorage,
    viewport: { w: window.innerWidth, h: window.innerHeight },
    applyTheme,
  });
  // The prototype applies the theme and is ready as soon as it loads; index.ts does this for the port.
  applyTheme(port.S);
  port.S.ready = true;
  const twin: Twin = {
    proto,
    port,
    run(name, ...args) {
      proto.call(name, ...args);
      callPort(port, name, args);
    },
    ask(name, ...args) {
      return {
        proto: snapshot(proto.call(name, ...args)),
        port: snapshot(callPort(port, name, args)),
      };
    },
    same() {
      expect(snapshot(port.S)).toEqual(snapshot(proto.S));
    },
    play(ms, step = 250) {
      for (let t = 0; t < ms; t += step) {
        vi.advanceTimersByTime(step);
        twin.same();
      }
    },
    confirmDialog() {
      // Same order as the shell's confirm button: close the dialog, then run its action.
      const runs = [dialogRun(proto.S), dialogRun(port.S)];
      twin.run("set", { dialog: null });
      for (const run of runs) run();
    },
  };
  return twin;
}
