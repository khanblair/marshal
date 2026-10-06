import type { UploadGoogleFileRequest } from "@marshal/protocol";
import { clip, MAX_TITLE_CHARS, titleOf } from "./export-text";

const EXTENSION = ".md";
const PATH_MARKS = /[\\/]+/g;

/** The note as a markdown file named after the card, or null when there is no note to save. */
export function noteUpload(
  card: { n: number; title: string },
  note: string,
): UploadGoogleFileRequest | null {
  if (!note.trim()) return null;
  const stem = titleOf(card).replace(PATH_MARKS, "-");
  const name = `${clip(stem, MAX_TITLE_CHARS - EXTENSION.length)}${EXTENSION}`;
  return { name, content: note, mimeType: "text/markdown" };
}
