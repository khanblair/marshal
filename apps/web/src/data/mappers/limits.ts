import type { LimitKind, LimitList } from "@marshal/protocol";

/**
 * The three ceilings one scope can have, as the store and the form hold them. Every field is
 * optional: a field that is absent means **no ceiling is set**, which is how the daemon ships (a
 * fresh install has no limits at all) and what an empty form field shows. A field that is present is
 * in the unit its own kind measures - whole dollars for `day` and `month`, whole cards for `awake`.
 *
 * It lives in the data layer, not in `mock/`, so the mapper never depends on the mock.
 */
interface Limits {
  day?: number;
  month?: number;
  awake?: number;
}

/** One ceiling per scope: `global` for the whole install, then a project id. */
export interface LimitsByScope {
  global: Limits;
  [pid: string]: Limits;
}

/** 1 dollar is this many micro-dollars on the wire, the unit every cost ceiling travels in. */
export const MICROS_PER_DOLLAR = 1_000_000;

/** A dollar amount as the wire's micro-dollars. The store keeps whole dollars, so this rounds. */
export const dollarsToMicros = (dollars: number): number => Math.round(dollars * MICROS_PER_DOLLAR);

/** A wire micro-dollar amount as the store's whole dollars. */
export const microsToWholeDollars = (micros: number): number =>
  Math.round(micros / MICROS_PER_DOLLAR);

/** A wire micro-dollar amount as plain dollars, cents and all, for the spend a screen shows. */
export const microsToDollars = (micros: number): number => micros / MICROS_PER_DOLLAR;

/** The form's own name for each kind: the settings screen has three fields per scope. */
export type LimitField = "day" | "month" | "awake";

/** The wire kind behind each form field, for a PUT or a DELETE. */
export const KIND_OF: Readonly<Record<LimitField, LimitKind>> = {
  day: "cost-day",
  month: "cost-month",
  awake: "awake",
};

const FIELD_OF: Readonly<Record<LimitKind, LimitField>> = {
  "cost-day": "day",
  "cost-month": "month",
  awake: "awake",
};

/** One field's value as the wire wants it: a cost field travels as micro-dollars, awake as a count. */
export const wireValueOf = (field: LimitField, value: number): number =>
  field === "awake" ? value : dollarsToMicros(value);

/** One field's wire value as the store holds it. */
function toFieldValue(kind: LimitKind, value: number): number {
  return kind === "awake" ? value : microsToWholeDollars(value);
}

/**
 * The daemon's whole list as the store holds it. `global` is always present, so a screen that reads
 * `S.limits.global` finds something; a kind the daemon did not send stays absent, which is a scope
 * with no ceiling of that kind.
 */
export function toLimitsByScope(list: LimitList): LimitsByScope {
  const byScope: LimitsByScope = { global: {} };
  for (const limit of list.limits) {
    const scope = byScope[limit.scope] ?? {};
    byScope[limit.scope] = scope;
    scope[FIELD_OF[limit.kind]] = toFieldValue(limit.kind, limit.value);
  }
  return byScope;
}
