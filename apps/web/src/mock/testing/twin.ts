/**
 * Drives the prototype store and the port side by side on one fake clock, so a test
 * can run the same calls on both and compare every field of `S` afterwards. Tests name
 * cards by key (`api#41`); `proto-shape.ts` translates to the prototype's numbers.
 */
import { expect, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import { forgetProject } from "~/sync/projects";
import { contextOf, createTestMarshal, MOCK_PERSON_SECTIONS } from "~/testing/test-store";
import { applyTheme } from "../dom/theme";
import type { Marshal } from "../marshal";
import { createProtoShape, withoutOrphans } from "./proto-shape";
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
  /** Calls `M[name](...args)` on both stores. Card keys become the prototype's numbers for it. */
  run(name: string, ...args: unknown[]): void;
  /** Calls a query on both stores and returns both results as plain JSON, in the prototype's shape. */
  ask(name: string, ...args: unknown[]): { proto: unknown; port: unknown };
  /**
   * Removes a project on both sides. The port's removal is a call to the daemon, so this does
   * what its success path does: forget the project (its cards, chats, and notices) and toast.
   */
  removeProject(id: string): void;
  /** The prototype's id of the card with this key. */
  protoId(key: string): number | undefined;
  /** A value read from the port (plain JSON), in the prototype's shape. */
  shape(value: unknown): unknown;
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
  const port = createTestMarshal({
    hash,
    storage: window.localStorage,
    viewport: { w: window.innerWidth, h: window.innerHeight },
    applyTheme,
    // The twin compares the mock against the prototype, so it pins the sections it exercises to the
    // mock: the mock still exists (it is deleted in Phase 13) and this suite is what guards it. Its
    // cards, their hold controls, their chat and activity, the project chats, and the Home feed are
    // the mock's own, whatever the register says.
    sections: {
      ...sectionStatus,
      S5a: "mock",
      S7c: "mock",
      S8a: "mock",
      S10: "mock",
      S17: "mock",
      S20: "mock",
      // The person too: the prototype's people, saved views, and profile are the mock's own.
      ...MOCK_PERSON_SECTIONS,
    },
  });
  // The prototype applies the theme and is ready as soon as it loads; index.ts does this for the port.
  applyTheme(port.S);
  port.S.ready = true;
  const twin: Twin = {
    proto,
    port,
    run(name, ...args) {
      const twinArgs = args.map(createProtoShape(port, proto).arg);
      proto.call(name, ...twinArgs);
      callPort(port, name, args);
    },
    ask(name, ...args) {
      const map = createProtoShape(port, proto);
      return {
        proto: snapshot(proto.call(name, ...args.map(map.arg))),
        port: map.shape(snapshot(callPort(port, name, args))),
      };
    },
    removeProject(id) {
      proto.call("removeProject", id);
      const project = port.proj(id);
      if (!project) return;
      forgetProject(contextOf(port), project);
      port.toast("Project removed");
    },
    protoId(key) {
      return createProtoShape(port, proto).protoId(key);
    },
    shape(value) {
      return createProtoShape(port, proto).shape(value);
    },
    same() {
      const shaped = createProtoShape(port, proto).shape(snapshot(port.S));
      expect(withoutOrphans(shaped)).toEqual(withoutOrphans(snapshot(proto.S)));
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
