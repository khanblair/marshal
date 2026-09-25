import type { DiffFile } from "~/mock";

export type DiffSign = " " | "+" | "-";

export interface DiffLineView {
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

const LARGE_FILE_LAST_LINE = 3;

/** What "Load diff" shows for a large file: the first lines of a generated go.sum. */
const LARGE_FILE_HUNK: DiffFile["hunks"] = [
  {
    h: "@@ -0,0 +1,1240 @@",
    lines: [
      ["+", 1, "cloud.google.com/go v0.115.0 h1:CnFSK6Xo3lDYRoBKEcAtia6VSC837/ZkJuRduSFnr14="],
      ["+", 2, "google.golang.org/grpc v1.66.0 h1:DibZuoBznOxbDQxRINckZcUvnCEvrW9pcWIE2yF9r1c="],
      ["+", LARGE_FILE_LAST_LINE, "..."],
    ],
  },
];

const signText = (sign: DiffSign): string => {
  if (sign === " ") return "";
  return sign === "-" ? MINUS : "+";
};

/** Hunks ready to draw. A large file only has lines once the user loaded it. */
export function hunkViews(file: DiffFile, loaded: boolean): DiffHunkView[] {
  const hunks = file.large && loaded ? LARGE_FILE_HUNK : file.hunks;
  return hunks.map((hunk) => ({
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

export const minus = (n: number): string => `${MINUS}${n}`;
