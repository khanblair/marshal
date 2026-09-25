import type { JSX } from "solid-js";

/**
 * Status flags drawn by hand in the design (the CUSTOM table in store.js), in
 * Lucide's stroke style. Each entry is the inner markup of a 24 by 24 icon.
 */
export const statusGlyphs = {
  "st-backlog": () => <circle cx="12" cy="12" r="8" stroke-dasharray="2.6 2.6" />,
  "st-planning": () => (
    <>
      <path d="M5 22V3" />
      <path d="M5 4h13l-2.6 4.5L18 13H5" />
    </>
  ),
  "st-working": () => (
    <>
      <path d="M5 22V3" />
      <path d="M5 4h13l-2.6 4.5L18 13H5z" fill="currentColor" />
    </>
  ),
  "st-done": () => (
    <>
      <path d="M5 22V3" />
      <rect x="5" y="4" width="14" height="9" rx=".5" />
      <path
        d="M5 4h3.5v4.5H5zM12 4h3.5v4.5H12zM8.5 8.5H12V13H8.5zM15.5 8.5H19V13h-3.5z"
        fill="currentColor"
        stroke="none"
      />
    </>
  ),
} satisfies Record<string, () => JSX.Element>;

export type StatusGlyphName = keyof typeof statusGlyphs;
