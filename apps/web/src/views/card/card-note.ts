import { type Card, M } from "~/mock";

const PATH_DEPTH = 2;

/** The note a card starts with, written from its title and package. */
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

/** The saved note, or the default one until the first save. Read it inside a memo. */
export const noteText = (card: Card): string => M.S.notes?.[card.id] ?? defaultNote(card);

/** Makes sure the card has a note in the store, as the design does when it draws the panel. */
export function ensureNote(card: Card): void {
  if (M.S.notes?.[card.id] != null) return;
  M.S.notes = { ...M.S.notes, [card.id]: defaultNote(card) };
}

export function saveNote(card: Card, text: string): void {
  M.S.notes = { ...M.S.notes, [card.id]: text };
  M.toast("Note saved");
}

/** Where the note lives in the Obsidian vault. */
export const notePath = (card: Card): string =>
  `vault/cards/${card.id}-${card.title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/-$/, "")}.md`;
