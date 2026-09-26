import type { DiffFile } from "~/mock";

type DiffSign = " " | "+" | "-";

interface DiffLineView {
  n: number;
  text: string;
  sign: string;
  bgClass: string;
  gutterClass: string;
  textClass: string;
}

export interface DiffHunkView {
  header: string;
  lines: DiffLineView[];
}

const MINUS = "−";
const LINE_LOOKS: Record<DiffSign, Pick<DiffLineView, "bgClass" | "gutterClass" | "textClass">> = {
  "+": {
    bgClass: "bg-diff-added-bg",
    gutterClass: "bg-diff-added-line",
    textClass: "text-diff-added-text",
  },
  "-": {
    bgClass: "bg-diff-removed-bg",
    gutterClass: "bg-diff-removed-line",
    textClass: "text-diff-removed-text",
  },
  " ": { bgClass: "bg-transparent", gutterClass: "bg-transparent", textClass: "text-primary" },
};

const signText = (sign: DiffSign): string => {
  if (sign === " ") return "";
  return sign === "-" ? MINUS : "+";
};

/** Hunks ready to draw: exactly the lines the file has, whatever its size. */
export function hunkViews(file: DiffFile): DiffHunkView[] {
  return file.hunks.map((hunk) => ({
    header: hunk.h,
    lines: hunk.lines.map(([sign, n, text]) => ({
      n,
      text,
      sign: signText(sign),
      ...LINE_LOOKS[sign],
    })),
  }));
}

const DEFAULT_OPEN_FILES = 2;

/** The expanded files: the user's choice, else the first two files that are not large. */
export function openPaths(
  diff: readonly DiffFile[],
  chosen: readonly string[] | null,
): Set<string> {
  if (chosen) return new Set(chosen);
  return new Set(
    diff
      .filter((file) => !file.large)
      .slice(0, DEFAULT_OPEN_FILES)
      .map((file) => file.path),
  );
}

export function toggled(open: ReadonlySet<string>, path: string): string[] {
  const next = new Set(open);
  if (next.has(path)) next.delete(path);
  else next.add(path);
  return [...next];
}

export const allOpen = (diff: readonly DiffFile[]): string[] => diff.map((file) => file.path);

/** The space between two files in the list: `gap-2.5`, which is 10 px. */
export const FILE_GAP_PX = 10;
/** A closed file: its header is `min-h-9` (36 px) inside a 1 px border on each side. */
const CLOSED_FILE_PX = 38;
/** One line of a hunk, or a hunk's header: `leading-5`. */
const LINE_PX = 20;
/** An open file is at least this many lines tall: the notice of a large one, or a short hunk. */
const MIN_OPEN_LINES = 3;

/**
 * How tall a file's row is thought to be until it is drawn and measured: closed, or open and as
 * tall as the lines it has now. The list draws only the files near the screen, and it places every
 * other one by this.
 */
export function estimateFileHeight(open: boolean, hunks: DiffFile["hunks"]): number {
  if (!open) return CLOSED_FILE_PX;
  const lines = hunks.reduce((count, hunk) => count + 1 + hunk.lines.length, 0);
  return CLOSED_FILE_PX + Math.max(lines, MIN_OPEN_LINES) * LINE_PX;
}
