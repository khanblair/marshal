/**
 * Scale tokens that do not change with the theme: type, space, radius, size,
 * motion, and layers. Theme colors live in themes.ts.
 * Values match the Claude Design prototype (design/Marshal.dc.html).
 */

export const fonts = {
  sans: '"Atkinson Hyperlegible Next",ui-sans-serif,system-ui,sans-serif',
  mono: '"Atkinson Hyperlegible Mono",ui-monospace,"SF Mono",Menlo,Consolas,monospace',
} as const;

/** Font sizes in px. Line height is set separately, because the design mixes both. */
export const textSizes = {
  micro: 10,
  badge: 11,
  caption: 12,
  small: 13,
  body: 14,
  lead: 15,
  subtitle: 16,
  title: 18,
  "view-title": 22,
  display: 24,
  tile: 28,
} as const;

/** One step of the spacing scale in px. `p-3` is 12 px, `gap-1.5` is 6 px. */
export const spacingStep = 4;

export const radii = {
  none: 0,
  "2xs": 2,
  xs: 3,
  sm: 5,
  "sm-plus": 6,
  md: 7,
  lg: 10,
  xl: 14,
  full: 9999,
} as const;

/** Named layout sizes in px, used for widths such as `w-sidebar`. */
export const sizes = {
  sidebar: 232,
  "sidebar-collapsed": 56,
  header: 48,
  column: 288,
  "detail-min": 480,
  "detail-max": 760,
  "chat-list": 280,
  "touch-min": 44,
} as const;

export const breakpoints = { sm: 640, md: 900, lg: 1200, xl: 1600 } as const;

export const durations = { instant: 80, fast: 140, base: 200, slow: 280 } as const;

export const easings = {
  standard: "cubic-bezier(0.2, 0, 0, 1)",
  enter: "cubic-bezier(0, 0, 0, 1)",
  exit: "cubic-bezier(0.4, 0, 1, 1)",
} as const;

/** Stacking order, using the numbers from the design. */
export const layers = {
  raised: 5,
  sticky: 10,
  bar: 20,
  banner: 50,
  menu: 100,
  detail: 150,
  side: 160,
  scrim: 200,
  dialog: 300,
  sheet: 300,
  palette: 400,
  toast: 500,
  tour: 650,
  onboarding: 700,
} as const;

/** Colors that stay the same in both themes. */
export const staticColors = {
  white: "#FFFFFF",
  "on-status": "#16191D",
  "scrim-side": "rgb(10 12 14 / 0.3)",
  "scrim-tour": "rgb(10 12 14 / 0.38)",
  "scrim-sheet": "rgb(10 12 14 / 0.4)",
  "scrim-dialog": "rgb(10 12 14 / 0.45)",
  "scrim-shadow": "rgb(0 0 0 / 0.25)",
  "tour-dim": "rgb(10 12 14 / 0.38)",
  /* Settings theme cards draw a small light or dark board, whatever the current theme. */
  "theme-preview-light-bg": "#F5F6F7",
  "theme-preview-light-line": "#C9CED4",
  "theme-preview-light-card": "#FFFFFF",
  "theme-preview-dark-bg": "#15181C",
  "theme-preview-dark-line": "#3A424B",
  "theme-preview-dark-card": "#1B1F24",
  "theme-preview-working": "#1E9E5A",
  "theme-preview-needs-you": "#F2B81F",
  /* The card panel Preview tab draws card #118 as a dark page and a light before shot, whatever the theme. */
  "card-preview-dark-side": "#1B1F24",
  "card-preview-dark-main": "#22272D",
  "card-preview-dark-block": "#3A424B",
  "card-preview-light-block": "#C9CED4",
} as const;

export const keyframes = {
  "m-pulse": "0%,100%{opacity:1}50%{opacity:.45}",
  "m-spin": "to{transform:rotate(360deg)}",
} as const;
