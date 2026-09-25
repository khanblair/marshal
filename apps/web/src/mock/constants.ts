import type { Card, CiState, Column, Status, ToneKind, ViewKey } from "./types";

export const MINUTE_MS = 60_000;
export const HOUR_MS = 3_600_000;
export const DAY_MS = 86_400_000;
/** Viewports narrower than this use the phone layout. */
export const PHONE_MAX_WIDTH_PX = 640;

export interface StatusInfo {
  label: string;
  icon: string;
  tone: string | null;
}

export const STATUS: Record<Status, StatusInfo> = {
  backlog: { label: "Backlog", icon: "st-backlog", tone: null },
  planning: { label: "Planning", icon: "st-planning", tone: "planning" },
  working: { label: "Working", icon: "st-working", tone: "working" },
  needs: { label: "Needs you", icon: "st-needs", tone: "needs-you" },
  review: { label: "In review", icon: "st-review", tone: "review" },
  ready: { label: "Ready to merge", icon: "st-ready", tone: "ready" },
  merging: { label: "Merging", icon: "st-ready", tone: "ready" },
  done: { label: "Done", icon: "st-done", tone: "done" },
};

export const COLUMNS: readonly Column[] = [
  "backlog",
  "planning",
  "working",
  "needs",
  "review",
  "ready",
  "done",
];

export const colOf = (s: Status): Column => (s === "merging" ? "ready" : s);

export const isColumn = (v: string): v is Column => (COLUMNS as readonly string[]).includes(v);

/** CSS color token for a status tone. A missing tone means the neutral backlog look. */
export function tone(t: string | null, k: ToneKind): string {
  if (!t) {
    if (k === "solid") return "var(--color-border-strong)";
    return k === "text" ? "var(--color-text-secondary)" : "var(--color-surface-sunken)";
  }
  return `var(--color-status-${t}-${k})`;
}

export interface CiInfo {
  label: string;
  icon: string;
  color: string;
}

export const CI: Record<CiState, CiInfo> = {
  queued: { label: "Queued", icon: "spinner", color: "var(--color-text-secondary)" },
  running: { label: "Running", icon: "spinner", color: "var(--color-text-secondary)" },
  passed: { label: "Passed", icon: "check", color: tone("working", "text") },
  failed: { label: "Failed", icon: "x", color: tone("danger", "text") },
  cancelled: { label: "Cancelled", icon: "slash", color: "var(--color-text-muted)" },
};

export const THINK: readonly string[] = ["Low", "Medium", "High", "Extra high"];
export const PERMS: readonly string[] = [
  "Ask",
  "Auto-accept edits",
  "Plan only",
  "Full auto",
  "Bypass permissions",
];
export const ROLE_NAMES: readonly string[] = [
  "Orchestrator",
  "Worker",
  "Reviewer",
  "Integrator",
  "Tester",
  "Docs writer",
  "Security checker",
  "UI checker",
];

export interface ViewInfo {
  key: ViewKey;
  label: string;
  icon: string;
}

export const VIEWS: readonly ViewInfo[] = [
  { key: "chat", label: "Chats", icon: "messages-square" },
  { key: "agents", label: "Agents", icon: "bot" },
  { key: "board", label: "Board", icon: "square-kanban" },
  { key: "list", label: "List", icon: "list" },
  { key: "timeline", label: "Timeline", icon: "gantt-chart" },
  { key: "calendar", label: "Calendar", icon: "calendar" },
];

/** A card has a live agent session unless it is in backlog, done, or asleep. */
export const isAwake = (c: Card): boolean =>
  c.state !== "backlog" && c.state !== "done" && !c.asleep;
