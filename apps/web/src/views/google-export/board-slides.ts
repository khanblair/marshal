import type { CreateGoogleSlidesRequest, GoogleSlide } from "@marshal/protocol";
import { clip, MAX_TITLE_CHARS, oneLine } from "./export-text";

/** One column of the board: its name and the titles of its cards, in the order to show them. */
export interface SlideColumn {
  label: string;
  titles: readonly string[];
}

export interface BoardSlidesInput {
  project: string;
  columns: readonly SlideColumn[];
}

/** What the daemon takes in one presentation and on one slide. */
export const MAX_SLIDES = 100;
export const MAX_BULLETS = 50;

const cards = (count: number): string => `${count} ${count === 1 ? "card" : "cards"}`;

/** The lines of one slide. Too many are cut, and the last line says how many were left out. */
function bulletsOf(lines: readonly string[]): string[] {
  if (lines.length <= MAX_BULLETS) return [...lines];
  const shown = MAX_BULLETS - 1;
  return [...lines.slice(0, shown), `and ${lines.length - shown} more`];
}

function titleSlide(input: BoardSlidesInput): GoogleSlide {
  const total = input.columns.reduce((sum, column) => sum + column.titles.length, 0);
  const counts = input.columns
    .filter((column) => column.titles.length > 0)
    .map((column) => `${oneLine(column.label)}: ${column.titles.length}`);
  return { title: oneLine(input.project), bullets: bulletsOf([cards(total), ...counts]) };
}

function columnSlide(column: SlideColumn): GoogleSlide {
  const lines = column.titles.map(oneLine).filter(Boolean);
  return {
    title: `${oneLine(column.label)} (${column.titles.length})`,
    bullets: bulletsOf(lines.length > 0 ? lines : ["No cards."]),
  };
}

/** A Google Slides presentation of a board: a title slide with the counts, then a slide per column. */
export function boardSlides(input: BoardSlidesInput): CreateGoogleSlidesRequest {
  const slides = [titleSlide(input), ...input.columns.map(columnSlide)].slice(0, MAX_SLIDES);
  return { title: clip(oneLine(`${input.project} board`), MAX_TITLE_CHARS), slides };
}
