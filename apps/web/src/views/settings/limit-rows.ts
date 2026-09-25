import { M } from "~/mock";

export type LimitKey = "day" | "month" | "awake";

export interface LimitSpec {
  key: LimitKey;
  label: string;
  /** A dollar limit. The awake limit counts cards. */
  money: boolean;
  step: string;
}

export type HelpTone = "danger" | "near" | "normal";

/** From this share of a limit on, the help text says the limit is near. */
const NEAR_LIMIT_RATIO = 0.8;

export const GLOBAL_SCOPE = "global";

/** The three limits every scope has, in the order the design shows them. */
export const LIMIT_SPECS: readonly LimitSpec[] = [
  { key: "day", label: "Daily cost limit", money: true, step: "0.5" },
  { key: "month", label: "Monthly cost limit", money: true, step: "5" },
  { key: "awake", label: "Awake card limit", money: false, step: "1" },
];

/** What a scope (`global` or a project id) uses now: dollars today or this month, or awake cards. */
export function limitUsed(scope: string, key: LimitKey): number {
  const pid = scope === GLOBAL_SCOPE ? null : scope;
  if (key === "awake") return M.awake(pid).length;
  const costs = M.costs(pid);
  return key === "day" ? costs.today : costs.month;
}

const usageText = (spec: LimitSpec, used: number): string =>
  spec.money ? `Used ${M.money(used)}` : `${used} awake now`;

/** The help line under a limit field and its tone: a bad value, over the limit, near it, or fine. */
export function limitHelp(
  spec: LimitSpec,
  used: number,
  value: number,
): { text: string; tone: HelpTone } {
  if (value <= 0) return { text: "Enter a limit above zero.", tone: "danger" };
  const ratio = used / value;
  if (ratio >= 1) return { text: `${usageText(spec, used)}. Over this limit.`, tone: "danger" };
  if (ratio >= NEAR_LIMIT_RATIO) {
    return { text: `${usageText(spec, used)}. Near this limit.`, tone: "near" };
  }
  return { text: usageText(spec, used), tone: "normal" };
}
