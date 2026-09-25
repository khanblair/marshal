/** WCAG contrast math and the list of token pairs the design must satisfy (ui-tokens.md section 2.9). */
import { type ThemeName, themes } from "./themes.ts";

const CHANNEL_KNEE = 0.03928;
const R_WEIGHT = 0.2126;
const G_WEIGHT = 0.7152;
const B_WEIGHT = 0.0722;
const FLARE = 0.05;

export const TEXT_MIN = 4.5;
export const NON_TEXT_MIN = 3;

function linear(channel: number): number {
  const c = channel / 255;
  return c <= CHANNEL_KNEE ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

function luminance(hex: string): number {
  const h = hex.replace("#", "");
  const [r, g, b] = [0, 2, 4].map((i) => Number.parseInt(h.slice(i, i + 2), 16));
  return R_WEIGHT * linear(r ?? 0) + G_WEIGHT * linear(g ?? 0) + B_WEIGHT * linear(b ?? 0);
}

export function contrastRatio(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return ((hi ?? 0) + FLARE) / ((lo ?? 0) + FLARE);
}

export interface ContrastPair {
  theme: ThemeName;
  fg: string;
  bg: string;
  min: number;
  ratio: number;
}

const SURFACES = [
  "color-canvas",
  "color-surface",
  "color-surface-sunken",
  "color-surface-raised",
  "color-surface-hover",
  "color-surface-selected",
];
const STATUSES = ["planning", "working", "needs-you", "review", "ready", "done", "danger"];
const TEXTS = ["color-text-primary", "color-text-secondary", "color-text-muted"];

type Rule = [fg: string, bg: string, min: number];

function rules(): Rule[] {
  const out: Rule[] = [];
  for (const s of SURFACES) {
    for (const t of TEXTS) out.push([t, s, TEXT_MIN]);
    out.push(["color-border-strong", s, NON_TEXT_MIN]);
    out.push(["color-ink", s, NON_TEXT_MIN]);
  }
  for (const st of STATUSES) {
    for (const s of SURFACES) {
      out.push([`color-status-${st}-solid`, s, NON_TEXT_MIN]);
      out.push([`color-status-${st}-text`, s, TEXT_MIN]);
    }
    out.push([`color-status-${st}-text`, `color-status-${st}-subtle`, TEXT_MIN]);
  }
  out.push(["color-text-inverse", "color-ink", TEXT_MIN]);
  out.push(["color-on-ink", "color-ink", TEXT_MIN]);
  out.push(["color-on-ink", "color-ink-hover", TEXT_MIN]);
  for (const kind of ["added", "removed"]) {
    for (const bg of ["bg", "line"]) {
      out.push([`color-diff-${kind}-text`, `color-diff-${kind}-${bg}`, TEXT_MIN]);
    }
  }
  for (const bg of ["color-bypass-bg", "color-bypass-stripe"])
    out.push(["color-bypass-text", bg, TEXT_MIN]);
  return out;
}

function chartRules(): Rule[] {
  const out: Rule[] = [];
  for (const s of ["color-surface", "color-canvas"]) {
    for (const i of [1, 2, 3, 4]) out.push([`chart-${i}`, s, NON_TEXT_MIN]);
    out.push(["chart-limit", s, NON_TEXT_MIN]);
    out.push(["chart-axis", s, TEXT_MIN]);
  }
  return out;
}

/** Every checked pair with its measured ratio, in both themes. */
export function measureAll(): ContrastPair[] {
  const pairs: ContrastPair[] = [];
  for (const theme of ["light", "dark"] as const) {
    const t = themes[theme];
    const token = (name: string) => t.colors[name] ?? t.charts[name];
    for (const [fg, bg, min] of [...rules(), ...chartRules()]) {
      const fgHex = token(fg);
      const bgHex = token(bg);
      if (fgHex && bgHex) pairs.push({ theme, fg, bg, min, ratio: contrastRatio(fgHex, bgHex) });
    }
  }
  return pairs;
}

export const pairKey = (p: Pick<ContrastPair, "theme" | "fg" | "bg">) =>
  `${p.theme}|${p.fg}|${p.bg}`;
