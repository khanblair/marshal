import { KIND_OF, type LimitField, type LimitsByScope, wireValueOf } from "~/data/mappers/limits";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";
import { applyLimitList } from "./limits";

/** The three fields a scope can have a ceiling for, in the order the form shows them. */
const FIELDS: readonly LimitField[] = ["day", "month", "awake"];

/** Every scope the store and the form name, so a ceiling the form cleared is found even with no project. */
const scopesOf = (a: LimitsByScope, b: LimitsByScope): string[] => [
  ...new Set([...Object.keys(a), ...Object.keys(b)]),
];

/**
 * Saves a form's ceilings on the daemon (section S26b). For each scope and field it compares what was
 * typed with what the store holds: a value present and different is a PUT, a ceiling that was set and
 * is now unset (the field was cleared) is a DELETE, and anything else is left alone. Each answer is
 * the whole list and is applied as it comes, and one sentence is shown when any of them landed.
 *
 * A refusal is shown by `ctx.optimistic` in the daemon's own words, the store keeps what it had, and
 * this returns false so the form keeps the person's edits.
 */
export async function saveLimits(ctx: Ctx, next: LimitsByScope): Promise<boolean> {
  const current = ctx.S.limits;
  let saved = false;
  let refused = false;
  for (const scope of scopesOf(current, next)) {
    for (const field of FIELDS) {
      const to = next[scope]?.[field];
      const from = current[scope]?.[field];
      if (to === from) continue;
      // A field the form cleared removes a ceiling the store had; one it never had is left alone.
      const value = to === undefined ? null : to;
      if (value === null && from === undefined) continue;
      if (await write(ctx, scope, field, value)) saved = true;
      else refused = true;
    }
  }
  if (refused) return false;
  if (saved) toast(ctx, "Limits saved");
  return true;
}

/** One ceiling's PUT or DELETE. The answer is the whole list, applied to the store on success. */
async function write(
  ctx: Ctx,
  scope: string,
  field: LimitField,
  value: number | null,
): Promise<boolean> {
  const api = ctx.env.data?.api;
  if (!api) return false;
  const kind = KIND_OF[field];
  try {
    const list = await ctx.optimistic({
      key: `limit:${scope}:${field}`,
      apply: () => undefined,
      request: () =>
        value === null
          ? api.deleteLimit(scope, kind)
          : api.setLimit(scope, kind, { value: wireValueOf(field, value) }),
      rollback: () => undefined,
    });
    applyLimitList(ctx, list);
    return true;
  } catch {
    return false;
  }
}
