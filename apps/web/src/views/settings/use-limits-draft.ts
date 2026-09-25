import { batch, createMemo, createSignal } from "solid-js";
import { M, type State } from "~/mock";
import { cloneJson, sameJson } from "./json";
import { GLOBAL_SCOPE, type LimitKey } from "./limit-rows";

type LimitsByScope = State["limits"];

/** Whether any of a scope's limits is zero or less, which the design will not save. */
const hasBadLimit = (scope: LimitsByScope[string] | undefined): boolean =>
  !!scope && (scope.day <= 0 || scope.month <= 0 || scope.awake <= 0);

/** Scopes whose day or month cost limit goes up. */
function raisedScopes(next: LimitsByScope): string[] {
  return Object.keys(next).filter((scope) => {
    const to = next[scope];
    const from = M.S.limits[scope];
    return !!to && !!from && (to.day > from.day || to.month > from.month);
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
      row[key] = +value;
      setEdited(next);
    },
    save(): void {
      if (unchanged()) return;
      const next = limits();
      if (Object.values(next).some(hasBadLimit)) {
        M.toast("Fix the limits marked in red before saving.");
        return;
      }
      const apply = () =>
        batch(() => {
          M.S.limits = cloneJson(next);
          discard();
          M.toast("Limits saved");
        });
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
