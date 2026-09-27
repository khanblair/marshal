import type { Note as WireNote, NoteAuthor } from "@marshal/protocol";
import { toMillis } from "./time";

/*
 * A card's note (section S14, docs/backend-checklist B7.4, N11, task 7.12): the wire's `Note`, kept
 * as the store already keeps a note's body (`M.S.notes`, a plain string, unchanged since the mock)
 * plus the daemon's own metadata the mock never had - where the file really lives in the vault, who
 * wrote it last, and when. The body and the metadata are stored separately (`M.S.notes[key]` and
 * `M.S.noteInfo[key]`) rather than replacing the body's shape with an object, so every place that
 * already reads a note's text as a plain string keeps working unchanged once S14 switches.
 */

/** The daemon's own half of a card's note: what the mock never had anything real to say about. */
export interface NoteInfo {
  /** Where the note lives in the vault, relative to its root (`<project>/cards/<n>-<title>.md`). */
  path: string;
  /** Who wrote it last. */
  author: NoteAuthor;
  /** When it was last saved, in ms on this device's clock, or null when nothing has been saved. */
  updatedAt: number | null;
}

/** One wire note, split into the body the store already shapes and the daemon's own metadata. */
export function toNoteInfo(note: WireNote): NoteInfo {
  return {
    path: note.path,
    author: note.author,
    updatedAt: note.updatedAt ? toMillis(note.updatedAt) : null,
  };
}
