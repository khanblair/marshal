import { type Card, type Column, M } from "~/mock";
import { noteText } from "~/views/card/card-note";
import { boardSheet } from "./board-sheet";
import { boardSlides, type SlideColumn } from "./board-slides";
import { cardDoc } from "./card-doc";
import { runExport } from "./export-run";
import { NOTE_LOADING, NOTE_MISSING, noteProblem, noteState } from "./note-state";
import { noteUpload } from "./note-upload";

/** The keys that say which export of which card or project is running. */
export const exportKey = (kind: "doc" | "note" | "sheet" | "slides", id: string): string =>
  `${kind}:${id}`;

const nameOf = (id: string): string => M.person(id)?.name ?? id;
const projectName = (pid: string): string => M.proj(pid)?.name ?? pid;

/** A project's cards, a board column at a time, in the order the board draws them. */
function columnsOf(pid: string): { col: Column; cards: Card[] }[] {
  const all = M.cardsOf(pid);
  return M.COLUMNS.map((col) => ({ col, cards: M.colCards(all, col) }));
}

/** Makes a Google Doc of a card, with its note and comments. */
export function exportCardToDoc(card: Card): Promise<boolean> {
  return runExport({
    key: exportKey("doc", card.id),
    service: "gdocs",
    make: async () => {
      const state = noteState(card);
      if (state === "loading") return { error: NOTE_LOADING };
      const note = state === "saved" ? noteText(card) : "";
      const project = projectName(card.p);
      return M.createGoogleDoc(
        cardDoc({ card, state: M.STATUS[card.state].label, project, note, nameOf }),
      );
    },
  });
}

/** Saves a card's note to the Drive folder as a markdown file. */
export function saveNoteToDrive(card: Card): Promise<boolean> {
  return runExport({
    key: exportKey("note", card.id),
    service: "gdrive",
    make: async () => {
      const upload = noteUpload(card, noteText(card));
      const problem = noteProblem(card);
      return problem || !upload ? { error: problem ?? NOTE_MISSING } : M.uploadGoogleFile(upload);
    },
  });
}

/** Makes a Google Sheet of every card on a project's board. */
export function exportBoardToSheet(pid: string): Promise<boolean> {
  const cards = columnsOf(pid).flatMap((column) => column.cards);
  return runExport({
    key: exportKey("sheet", pid),
    service: "gsheets",
    make: () =>
      M.createGoogleSheet(
        boardSheet({
          project: projectName(pid),
          cards,
          stateLabel: (state) => M.STATUS[state].label,
          columnLabel: (state) => M.STATUS[M.colOf(state)].label,
          nameOf,
        }),
      ),
  });
}

/** Makes a Google Slides presentation of a project's board: a slide for each column. */
export function exportBoardToSlides(pid: string): Promise<boolean> {
  const columns: SlideColumn[] = columnsOf(pid).map(({ col, cards }) => ({
    label: M.STATUS[col].label,
    titles: cards.map((card) => card.title),
  }));
  return runExport({
    key: exportKey("slides", pid),
    service: "gslides",
    make: () => M.createGoogleSlides(boardSlides({ project: projectName(pid), columns })),
  });
}
