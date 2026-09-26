import {
  type CardDiff,
  type ChangedFile,
  DiffLineKindAdded,
  DiffLineKindRemoved,
  type DiffHunk as WireDiffHunk,
  type DiffLine as WireDiffLine,
} from "@marshal/protocol";
import { isDaemon } from "~/data/sections";
import { type Ctx, sectionsOf } from "~/mock/context";
import { diffFor, LARGE_FILE_SAMPLE } from "~/mock/seed/diffs";
import type { Card, DiffFile } from "~/mock/types";

/**
 * Section S11: a card's diff (docs/backend-checklist.md B2.9). `GET /v1/cards/{id}/diff` answers
 * the changed files and their counts, with no hunks; a file's own hunks are read separately, from
 * `GET /v1/cards/{id}/diff/{path}`, once the screen opens it. Until S11 is switched, and for any
 * card the daemon does not have (`card.daemonId` unset), both halves come from the mock's own
 * `diffFor` instead, which already carries every file's hunks up front (all but its one large
 * file, whose lines are a sample).
 */

function signOf(kind: WireDiffLine["kind"]): DiffFile["hunks"][number]["lines"][number][0] {
  if (kind === DiffLineKindRemoved) return "-";
  if (kind === DiffLineKindAdded) return "+";
  return " ";
}

const lineNumberOf = (line: WireDiffLine): number =>
  line.kind === DiffLineKindRemoved ? line.oldLine : line.newLine;

function storedHunk(hunk: WireDiffHunk): DiffFile["hunks"][number] {
  return {
    h: hunk.header,
    lines: hunk.lines.map((line) => [signOf(line.kind), lineNumberOf(line), line.text]),
  };
}

function storedFile(file: ChangedFile): DiffFile {
  return {
    path: file.path,
    add: file.additions,
    del: file.deletions,
    large: file.large,
    hunks: [],
  };
}

/** The daemon it is the diff of, when S11 is switched, a live connection answers it, and the card
 * is a real one the daemon knows (a mock-only card has no diff to fetch). */
function daemonDiff(
  ctx: Ctx,
  card: Card,
): { api: NonNullable<Ctx["env"]["data"]>["api"]; id: string } | null {
  const api = ctx.env.data?.api;
  if (!isDaemon("S11", sectionsOf(ctx.env)) || !api || !card.daemonId) return null;
  return { api, id: card.daemonId };
}

/** A card's changed files with their counts, in the shape the Diff tab draws (no hunks yet). */
export async function loadCardDiff(ctx: Ctx, card: Card): Promise<DiffFile[]> {
  const daemon = daemonDiff(ctx, card);
  if (!daemon) return diffFor(card);
  const wire: CardDiff = await daemon.api.diff(daemon.id);
  return wire.files.map(storedFile);
}

/** One file's hunks, loaded when the Diff tab opens it. */
export async function loadFileHunks(
  ctx: Ctx,
  card: Card,
  path: string,
): Promise<DiffFile["hunks"]> {
  const daemon = daemonDiff(ctx, card);
  if (!daemon) {
    const file = diffFor(card).find((one) => one.path === path);
    return file?.large ? LARGE_FILE_SAMPLE : (file?.hunks ?? []);
  }
  const wire = await daemon.api.fileHunks(daemon.id, path);
  return wire.hunks.map(storedHunk);
}
