import type { CreateGoogleDocRequest } from "@marshal/protocol";
import { renderMarkdown } from "~/features/markdown/render";
import type { Card, Checklist, Comment } from "~/mock";
import { escapeHtml, htmlText, minuteOf, titleOf } from "./export-text";

export interface CardDocInput {
  card: Pick<Card, "n" | "title" | "doing" | "checklists" | "comments">;
  /** What the board calls the card's state, such as "In review". */
  state: string;
  project: string;
  /** The saved note, as markdown. Empty when the card has none. */
  note: string;
  /** The name of the person with this id. */
  nameOf: (id: string) => string;
}

const ticked = (done: boolean): string => (done ? "☑" : "☐");

function checklistHtml(list: Checklist): string {
  const heading = `<h3>${escapeHtml(list.title.trim() || "Checklist")}</h3>`;
  if (list.items.length === 0) return `${heading}<p>No items.</p>`;
  const items = list.items.map(
    (item) => `<li>${ticked(item.done)} ${escapeHtml(item.text.trim())}</li>`,
  );
  return `${heading}<ul>${items.join("")}</ul>`;
}

function commentHtml(comment: Comment, nameOf: (id: string) => string): string {
  const when = minuteOf(comment.ts);
  const byline = [
    `<strong>${escapeHtml(nameOf(comment.author))}</strong>`,
    when && escapeHtml(when),
  ];
  return `<p>${byline.filter(Boolean).join(" · ")}<br>${htmlText(comment.text)}</p>`;
}

/** The parts of the document after its title, each one left out when the card has nothing for it. */
function sections(input: CardDocInput): string[] {
  const { card, note } = input;
  const out: string[] = [];
  if (card.doing.trim()) out.push(`<p>${htmlText(card.doing)}</p>`);
  out.push(
    `<p><strong>State:</strong> ${escapeHtml(input.state)}<br><strong>Project:</strong> ${escapeHtml(input.project)}</p>`,
  );
  if (card.checklists.length > 0) {
    out.push("<h2>Checklists</h2>", ...card.checklists.map(checklistHtml));
  }
  // The note is markdown from outside, so it goes through the app's own sanitizing renderer.
  if (note.trim()) out.push("<h2>Note</h2>", renderMarkdown(note));
  if (card.comments.length > 0) {
    out.push("<h2>Comments</h2>", ...card.comments.map((c) => commentHtml(c, input.nameOf)));
  }
  return out;
}

/**
 * A Google Doc made from a card: its title, what it is doing now, its state and project, its
 * checklists, its note and its comments. Every piece of card text is escaped, so none of it can
 * act as markup.
 */
export function cardDoc(input: CardDocInput): CreateGoogleDocRequest {
  const title = titleOf(input.card);
  return { title, html: [`<h1>${escapeHtml(title)}</h1>`, ...sections(input)].join("\n") };
}
