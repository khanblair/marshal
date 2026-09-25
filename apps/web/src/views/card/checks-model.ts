import type { CheckState } from "~/mock";

export interface CheckLook {
  icon: string;
  colorClass: string;
  label: string;
}

/** How an acceptance check looks in each state. */
export const CHECK_LOOKS: Record<CheckState, CheckLook> = {
  passed: { icon: "check", colorClass: "text-status-working-text", label: "Passed" },
  failed: { icon: "x", colorClass: "text-status-danger-text", label: "Failed" },
  running: { icon: "spinner", colorClass: "text-secondary", label: "Running" },
  pending: { icon: "circle-dashed", colorClass: "text-muted", label: "Pending" },
};

const PERCENT = 100;

/** Done over total for one checklist, as a whole percent. */
export function percentDone(done: number, total: number): number {
  return total ? Math.round((done / total) * PERCENT) : 0;
}
