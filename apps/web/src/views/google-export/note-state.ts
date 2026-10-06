import { isDaemon } from "~/data/sections";
import { type Card, M } from "~/mock";
import { noteText } from "~/views/card/card-note";

/** Whether a card's note can be used yet: still being read, never written, or there to use. */
export type NoteState = "loading" | "none" | "saved";

export const NOTE_LOADING = "The note is still loading. Try again in a moment.";
export const NOTE_MISSING = "This card has no note yet.";

/**
 * Read it where `noteText` is read. Once notes are the daemon's, a card nothing was saved for has
 * only the starter note it writes itself, which is not worth sending anywhere.
 */
export function noteState(card: Card): NoteState {
  const onDaemon = isDaemon("S14");
  if (onDaemon && M.S.notes?.[card.id] === undefined) return "loading";
  if (!noteText(card).trim()) return "none";
  const info = M.S.noteInfo?.[card.id];
  return onDaemon && info && info.updatedAt === null ? "none" : "saved";
}

/** The sentence for a note that cannot be used, or null when it can. */
export function noteProblem(card: Card): string | null {
  const state = noteState(card);
  if (state === "loading") return NOTE_LOADING;
  return state === "none" ? NOTE_MISSING : null;
}
