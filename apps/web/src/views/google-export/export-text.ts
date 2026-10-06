import { escapeHtml } from "~/features/markdown/render";

export { escapeHtml };

/** The longest title or file name the daemon takes. */
export const MAX_TITLE_CHARS = 200;

const ELLIPSIS = "…";
const DATE_CHARS = 10;
const MINUTE_CHARS = 16;

/** Text on one line: every run of spaces, line breaks and control characters becomes one space. */
export const oneLine = (text: string): string => text.replace(/[\p{Cc}\s]+/gu, " ").trim();

/** At most `max` characters, counted as people count them, with an ellipsis where it was cut. */
export function clip(text: string, max: number): string {
  const chars = Array.from(text);
  return chars.length <= max ? text : `${chars.slice(0, max - 1).join("")}${ELLIPSIS}`;
}

/** A card's name for a file: its title on one line and within the limit, else `Card #<n>`. */
export function titleOf(card: { n: number; title: string }): string {
  return clip(oneLine(card.title), MAX_TITLE_CHARS) || `Card #${card.n}`;
}

const isoOf = (ms: number | null | undefined): string =>
  ms != null && Number.isFinite(ms) ? new Date(ms).toISOString() : "";

/** A day as `2026-10-06` in UTC, or nothing for a time that is not there. */
export const dayOf = (ms: number | null | undefined): string => isoOf(ms).slice(0, DATE_CHARS);

/** A minute as `2026-10-06 14:05 UTC`, or nothing for a time that is not there. */
export function minuteOf(ms: number | null | undefined): string {
  const iso = isoOf(ms);
  return iso ? `${iso.slice(0, MINUTE_CHARS).replace("T", " ")} UTC` : "";
}

/** Text as HTML with its line breaks kept, every character that means something escaped. */
export const htmlText = (text: string): string => escapeHtml(text.trim()).replace(/\r?\n/g, "<br>");
