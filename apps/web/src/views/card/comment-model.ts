import { type Attachment, type Card, type Comment, M } from "~/mock";

export interface CommentFileView {
  name: string;
  icon: string;
  meta: string;
  href: string;
  isLink: boolean;
  /** Set when the attachment has nothing to open, so a click only shows a toast. */
  placeholder: boolean;
}

export interface CommentView {
  id: string;
  isAgent: boolean;
  name: string;
  initials: string;
  when: string;
  full: string;
  readShow: boolean;
  text: string;
  images: { name: string; src: string }[];
  files: CommentFileView[];
  mine: boolean;
}

/** The signed-in user. Only their own comments can be deleted. */
const ME = "ada";
const INITIALS_MAX = 2;

export const initialsOf = (name: string): string =>
  name
    .split(" ")
    .map((part) => part[0])
    .join("")
    .slice(0, INITIALS_MAX);

const fileView = (file: Attachment): CommentFileView => ({
  name: file.name,
  icon: file.kind === "link" ? "link" : "file-text",
  meta: file.size ?? "",
  href: file.url || file.src || "#",
  isLink: file.kind === "link",
  placeholder: !file.url && !file.src,
});

function commentView(card: Card, comment: Comment): CommentView {
  const isAgent = comment.author === "agent";
  const person = isAgent ? undefined : M.person(comment.author);
  return {
    id: comment.id,
    isAgent,
    name: isAgent ? `${card.agent} agent` : (person?.name ?? "Someone"),
    initials: person ? initialsOf(person.name) : "",
    when: M.rel(comment.ts),
    full: M.full(comment.ts),
    readShow: !isAgent && comment.read && card.state !== "backlog",
    text: comment.text,
    images: comment.att
      .filter((att) => att.kind === "image")
      .map((att) => ({ name: att.name, src: att.src ?? "" })),
    files: comment.att.filter((att) => att.kind !== "image").map(fileView),
    mine: comment.author === ME,
  };
}

/** Comments newest first, as the design lists them. Read it inside a memo. */
export const commentViews = (card: Card): CommentView[] =>
  card.comments
    .slice()
    .reverse()
    .map((comment) => commentView(card, comment));

const MEGABYTE = 1e6;
const KILOBYTE = 1e3;

const sizeLabel = (bytes: number): string =>
  bytes > MEGABYTE
    ? `${(bytes / MEGABYTE).toFixed(1)} MB`
    : `${Math.max(1, Math.round(bytes / KILOBYTE))} KB`;

/** Attachments for the files the user picked. Images and files get a preview address. */
export function attachmentsFrom(files: FileList | null): Attachment[] {
  return Array.from(files ?? []).map((file) => ({
    kind: file.type.startsWith("image/") ? "image" : "file",
    name: file.name,
    size: sizeLabel(file.size),
    src: URL.createObjectURL(file),
  }));
}

/** A link the user typed as an attachment. A missing scheme becomes https. */
export function linkAttachment(value: string): Attachment {
  const url = /^https?:/.test(value) ? value : `https://${value}`;
  return { kind: "link", name: url.replace(/^https?:\/\//, ""), url };
}
