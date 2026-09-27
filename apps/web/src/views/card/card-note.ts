import { isDaemon } from "~/data/sections";
import { type Card, M } from "~/mock";

const PATH_DEPTH = 2;

/**
 * A card's note: one file per card in the Obsidian vault (section S14, docs/backend-checklist B7.4,
 * N11, task 7.12). While S14 is still the mock's, this module makes up a starter note and a guessed
 * vault path the same way the prototype always did; once it is the daemon's, the note - and the
 * `noteInfo` metadata `sync/card-session.ts` reads alongside it - come from the vault for real, and
 * this module's job shrinks to reading and writing the store's own fields, plainly.
 */

/** True once card notes are the daemon's, the same check every S14-aware read below makes. */
const onDaemon = (): boolean => isDaemon("S14");

/** The note a card starts with, written from its title and package. Mock-only: the daemon writes
 * its own placeholder (`memory.placeholderNote`) and this module never invents one to replace it. */
export function defaultNote(card: Card): string {
  const project = M.proj(card.p);
  const area = card.pkg || (M.filesFor(card)[0] ?? "").split("/").slice(0, PATH_DEPTH).join("/");
  return [
    `# ${card.title}`,
    "",
    `Goal: ${card.title.toLowerCase()}.`,
    "",
    "Decisions",
    `- Keep the change inside ${area}`,
    "- Add tests before pushing",
    "",
    "Links",
    `[[${project?.name}]]  [[lessons/ci-flaky-tests]]`,
  ].join("\n");
}

/**
 * The saved note. Once S14 is the daemon's, this reads exactly what `readOpenCard` last fetched (or
 * an empty string before it answers, for the instant a card first opens) and never the mock's
 * made-up default - a made-up note is the one thing a real vault must never be shown as holding.
 * Read it inside a memo.
 */
export const noteText = (card: Card): string =>
  M.S.notes?.[card.id] ?? (onDaemon() ? "" : defaultNote(card));

/**
 * Makes sure the card has a note in the store, as the design does when it draws the panel. Once S14
 * is the daemon's, this does nothing: `readOpenCard` is what fills the store, and a note in the
 * vault must appear only from a real save, never from opening the card's panel (the daemon's own
 * `Note`'s "reading never writes" rule, `internal/memory/notes.go`).
 */
export function ensureNote(card: Card): void {
  if (onDaemon()) return;
  if (M.S.notes?.[card.id] != null) return;
  M.S.notes = { ...M.S.notes, [card.id]: defaultNote(card) };
}

/**
 * Saves a card's note. Once S14 is the daemon's, this writes through the daemon
 * (`sync/notes.ts`'s `saveCardNote`), which is what keeps the store and the vault file in
 * agreement; the mock keeps its own plain assignment.
 */
export function saveNote(card: Card, text: string): void {
  if (onDaemon()) {
    void M.saveCardNote(card.id, text);
    return;
  }
  M.S.notes = { ...M.S.notes, [card.id]: text };
  M.toast("Note saved");
}

/**
 * Where the note lives in the Obsidian vault. Once S14 is the daemon's and the card has been read
 * (`readOpenCard` fills `noteInfo`), this is the real, project-scoped path
 * (`<project>/cards/<n>-<title>.md`) the daemon answered; before that, and always on the mock, it
 * is this module's own guess, which - unlike the daemon's - drops the project segment (the mock's
 * long-standing simplification that migration 0019's own header comment calls out as the side that
 * is slightly wrong).
 */
export function notePath(card: Card): string {
  const info = M.S.noteInfo?.[card.id];
  if (info) return info.path;
  return `vault/cards/${card.n}-${card.title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/-$/, "")}.md`;
}
