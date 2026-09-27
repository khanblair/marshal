import { batch, createMemo, createSignal } from "solid-js";
import type { LimitsByScope } from "~/data/mappers/limits";
import { M } from "~/mock";
import { cloneJson, sameJson } from "./json";
import { GLOBAL_SCOPE, type LimitKey } from "./limit-rows";

/** Whether any of a scope's set ceilings is zero or less, which the design will not save. */
const hasBadLimit = (scope: LimitsByScope[string] | undefined): boolean =>
  !!scope &&
  [scope.day, scope.month, scope.awake].some((value) => value !== undefined && value <= 0);

/** Scopes whose day or month cost ceiling goes up, with both the old and the new ceiling set. */
function raisedScopes(next: LimitsByScope): string[] {
  return Object.keys(next).filter((scope) => {
    const to = next[scope];
    const from = M.S.limits[scope];
    if (!to || !from) return false;
    const dayUp = to.day !== undefined && from.day !== undefined && to.day > from.day;
    const monthUp = to.month !== undefined && from.month !== undefined && to.month > from.month;
    return dayUp || monthUp;
  });
}

const scopeName = (scope: string): string =>
  scope === GLOBAL_SCOPE ? "all projects" : (M.proj(scope)?.name ?? scope);

/** The limits form's edits, kept while the section is switched away. */
export function createLimitsDraft() {
  const [edited, setEdited] = createSignal<LimitsByScope | null>(null);
  const limits = createMemo(() => edited() ?? cloneJson(M.S.limits));
  const unchanged = () => sameJson(limits(), M.S.limits);
  const discard = () => setEdited(null);
  return {
    limits,
    unchanged,
    discard,
    set(scope: string, key: LimitKey, value: string): void {
      const next = cloneJson(limits());
      const row = next[scope];
      if (!row) return;
      // An emptied field means no ceiling at all, not a zero; anything else is the typed number.
      row[key] = value.trim() === "" ? undefined : +value;
      setEdited(next);
    },
    save(): void {
      if (unchanged()) return;
      const next = limits();
      if (Object.values(next).some(hasBadLimit)) {
        M.toast("Fix the limits marked in red before saving.");
        return;
      }
      const apply = (): void => {
        // The daemon owns the limits once S26b is switched: it answers each PUT and DELETE, and the
        // form keeps the person's edits until it says yes. The mock saves at once.
        if (M.limitsOnDaemon()) {
          void M.saveLimits(next).then((ok) => {
            if (ok && sameJson(limits(), next)) discard();
          });
          return;
        }
        batch(() => {
          M.S.limits = cloneJson(next);
          discard();
          M.toast("Limits saved");
        });
      };
      const raised = raisedScopes(next);
      if (raised.length === 0) {
        apply();
        return;
      }
      M.confirm({
        title: "Raise cost limit",
        message: `Agents can spend more before Marshal pauses them in ${raised.map(scopeName).join(" and ")}.`,
        action: "Raise cost limit",
        run: apply,
      });
    },
  };
}

export type LimitsDraft = ReturnType<typeof createLimitsDraft>;
