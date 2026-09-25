/**
 * Color pairs in the Claude Design prototype that are below the contrast rules in
 * ui-tokens.md section 2.9. The owner accepted the design's exact colors on
 * 2026-09-24, so these stay until the design changes. Format: theme|foreground|background.
 * A new failing pair, or a listed pair that passes, fails `pnpm check`.
 */
export const contrastExceptions: readonly string[] = [
  "light|color-border-strong|color-canvas",
  "light|color-border-strong|color-surface",
  "light|color-text-muted|color-surface-sunken",
  "light|color-border-strong|color-surface-sunken",
  "light|color-border-strong|color-surface-raised",
  "light|color-text-muted|color-surface-hover",
  "light|color-border-strong|color-surface-hover",
  "light|color-text-muted|color-surface-selected",
  "light|color-border-strong|color-surface-selected",
  "light|color-status-planning-solid|color-canvas",
  "light|color-status-planning-solid|color-surface-sunken",
  "light|color-status-planning-solid|color-surface-hover",
  "light|color-status-planning-solid|color-surface-selected",
  "light|color-status-working-solid|color-surface-selected",
  "light|color-status-needs-you-solid|color-canvas",
  "light|color-status-needs-you-solid|color-surface-sunken",
  "light|color-status-needs-you-solid|color-surface-hover",
  "light|color-status-needs-you-solid|color-surface-selected",
  "light|color-status-ready-solid|color-surface-selected",
  "light|chart-3|color-surface",
  "light|chart-4|color-surface",
  "light|chart-3|color-canvas",
  "light|chart-4|color-canvas",
  "dark|color-border-strong|color-canvas",
  "dark|color-border-strong|color-surface",
  "dark|color-border-strong|color-surface-sunken",
  "dark|color-text-muted|color-surface-raised",
  "dark|color-border-strong|color-surface-raised",
  "dark|color-text-muted|color-surface-hover",
  "dark|color-border-strong|color-surface-hover",
  "dark|color-text-muted|color-surface-selected",
  "dark|color-border-strong|color-surface-selected",
  "dark|color-status-planning-solid|color-surface-selected",
  "dark|chart-4|color-surface",
  "dark|chart-4|color-canvas",
];
