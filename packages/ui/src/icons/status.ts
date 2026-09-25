/**
 * Card states and their color families, from the STATUS table and the `tone`
 * helper in design/store.js. Class names are written out in full so Tailwind
 * finds them.
 */
import type { IconName } from "./icon-names";

/** Card states. `merging` shows in the Ready to merge column. */
export type StatusKey =
  | "backlog"
  | "planning"
  | "working"
  | "needs"
  | "review"
  | "ready"
  | "merging"
  | "done";

/** A status color family, or `neutral` for states without one (backlog). */
export type ToneKey =
  | "neutral"
  | "planning"
  | "working"
  | "needs-you"
  | "review"
  | "ready"
  | "done"
  | "danger";

const STATUS_TONES: Record<StatusKey, ToneKey> = {
  backlog: "neutral",
  planning: "planning",
  working: "working",
  needs: "needs-you",
  review: "review",
  ready: "ready",
  merging: "ready",
  done: "done",
};

const STATUS_ICONS: Record<StatusKey, IconName> = {
  backlog: "st-backlog",
  planning: "st-planning",
  working: "st-working",
  needs: "st-needs",
  review: "st-review",
  ready: "st-ready",
  merging: "st-ready",
  done: "st-done",
};

export function isStatusKey(value: string): value is StatusKey {
  return Object.hasOwn(STATUS_TONES, value);
}

/** Color family for a state. `danger` is accepted too, as `m-dot` does. */
export function statusTone(state: string): ToneKey {
  if (state === "danger") return "danger";
  return isStatusKey(state) ? STATUS_TONES[state] : "neutral";
}

/** Icon for a state, as in STATUS[state].icon. Unknown states get the backlog circle. */
export function statusIcon(state: string): IconName {
  return isStatusKey(state) ? STATUS_ICONS[state] : "st-backlog";
}

/** Solid fill: dots and bars. Neutral uses the strong border color. */
export const toneSolidBg: Record<ToneKey, string> = {
  neutral: "bg-border-strong",
  planning: "bg-status-planning-solid",
  working: "bg-status-working-solid",
  "needs-you": "bg-status-needs-you-solid",
  review: "bg-status-review-solid",
  ready: "bg-status-ready-solid",
  done: "bg-status-done-solid",
  danger: "bg-status-danger-solid",
};

/** Solid color for icons on surfaces, as `tone(t, 'solid')`. */
export const toneSolidText: Record<ToneKey, string> = {
  neutral: "text-border-strong",
  planning: "text-status-planning-solid",
  working: "text-status-working-solid",
  "needs-you": "text-status-needs-you-solid",
  review: "text-status-review-solid",
  ready: "text-status-ready-solid",
  done: "text-status-done-solid",
  danger: "text-status-danger-solid",
};

/** Card icon color, as `deco(card).iconColor`: like the solid, but backlog is muted. */
export const toneIconText: Record<ToneKey, string> = {
  ...toneSolidText,
  neutral: "text-muted",
};

/** Readable text color, as `tone(t, 'text')`. */
export const toneText: Record<ToneKey, string> = {
  neutral: "text-secondary",
  planning: "text-status-planning-text",
  working: "text-status-working-text",
  "needs-you": "text-status-needs-you-text",
  review: "text-status-review-text",
  ready: "text-status-ready-text",
  done: "text-status-done-text",
  danger: "text-status-danger-text",
};

/** Tinted background, as `tone(t, 'subtle')`. */
export const toneSubtleBg: Record<ToneKey, string> = {
  neutral: "bg-surface-sunken",
  planning: "bg-status-planning-subtle",
  working: "bg-status-working-subtle",
  "needs-you": "bg-status-needs-you-subtle",
  review: "bg-status-review-subtle",
  ready: "bg-status-ready-subtle",
  done: "bg-status-done-subtle",
  danger: "bg-status-danger-subtle",
};
