/**
 * Test oracle: runs the design prototype's store (design/store.js) in jsdom so tests
 * can compare the port against the original, value for value.
 */
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";

/** The prototype's `window.M`, seen from tests. */
export interface ProtoM {
  S: unknown;
  /** Own keys of the prototype's `window.M`. */
  keys: string[];
  call(name: string, ...args: unknown[]): unknown;
}

/** Walks up from the test's working directory to the repo's design/store.js. */
function storePath(): string {
  let dir = process.cwd();
  while (!existsSync(join(dir, "design", "store.js"))) {
    const up = dirname(dir);
    if (up === dir) throw new Error("design/store.js not found above the working directory");
    dir = up;
  }
  return join(dir, "design", "store.js");
}

function takeWindowM(): Record<string, unknown> {
  const m: unknown = Reflect.get(window, "M");
  Reflect.deleteProperty(window, "M");
  if (typeof m !== "object" || m === null) throw new Error("design/store.js did not set window.M");
  return m as Record<string, unknown>;
}

/** Loads a fresh prototype store. Set up fake timers first so both stores share one clock. */
export function loadPrototype(hash = "#nosim"): ProtoM {
  Reflect.deleteProperty(window, "M");
  window.location.hash = hash;
  const src = readFileSync(storePath(), "utf8");
  new Function(src)();
  const m = takeWindowM();
  return {
    S: m.S,
    keys: Object.keys(m),
    call(name, ...args) {
      const fn = m[name];
      if (typeof fn !== "function") throw new Error(`prototype M.${name} is not a function`);
      return fn(...args);
    },
  };
}

/** Plain JSON copy: drops functions and undefined, invokes getters. */
export const snapshot = (value: unknown): unknown => JSON.parse(JSON.stringify(value) ?? "null");
