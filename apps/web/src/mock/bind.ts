import { batch } from "solid-js";
import type { Ctx } from "./context";

type CtxFn = (ctx: Ctx, ...args: never[]) => unknown;

/** The functions with their leading `ctx` argument filled in. */
export type Bound<T extends Record<string, CtxFn>> = {
  [K in keyof T]: T[K] extends (ctx: Ctx, ...args: infer A) => infer R ? (...args: A) => R : never;
};

function bindAll<T extends Record<string, CtxFn>>(
  fns: T,
  wrap: (fn: CtxFn) => (...args: never[]) => unknown,
): Bound<T> {
  const out: Record<string, unknown> = {};
  for (const [name, fn] of Object.entries(fns)) out[name] = wrap(fn);
  return out as Bound<T>;
}

/** Binds read-only queries to one store instance. */
export const bindQueries = <T extends Record<string, CtxFn>>(ctx: Ctx, fns: T): Bound<T> =>
  bindAll(
    fns,
    (fn) =>
      (...args) =>
        fn(ctx, ...args),
  );

/**
 * Binds actions to one store instance. Each call applies its writes as one update,
 * the way one prototype action ended in one `emit()`.
 */
export const bindActions = <T extends Record<string, CtxFn>>(ctx: Ctx, fns: T): Bound<T> =>
  bindAll(
    fns,
    (fn) =>
      (...args) =>
        batch(() => fn(ctx, ...args)),
  );
