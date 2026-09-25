import type { CardKey } from "../card-key";
import { isAwake } from "../constants";
import type { Ctx } from "../context";
import { addAct, later, live, toast } from "../engine";
import { takeCk } from "../ids";
import { card, person } from "../selectors";
import type { Attachment, Comment } from "../types";

const ME = "ada";
/** An awake agent reads a new comment after this long... */
const READ_MS = 1200;
/** ...and answers a question or a mention this long after reading it. */
const ANSWER_MS = 1600;

const linksIn = (text: string): Attachment[] =>
  (text.match(/https?:\/\/[^\s)]+/g) || []).map((url) => ({
    kind: "link",
    name: url.replace(/^https?:\/\//, ""),
    url,
  }));

const asksAgent = (text: string): boolean => /@agent|@/.test(text) || /\?$/.test(text.trim());

function agentAnswer(ctx: Ctx, cid: CardKey, cm: Comment): void {
  const c = card(ctx, cid);
  if (!c) return;
  const n = cm.att.length;
  const read = n ? ` and read the ${n}${n === 1 ? " attachment" : " attachments"}` : "";
  c.comments.push({
    id: `co${takeCk(ctx.ids)}`,
    author: "agent",
    text: `Got it. I added this to the plan for my next turn${read}.`,
    ts: Date.now(),
    att: [],
    read: true,
  });
}

/** Posts a comment. Links in the text become attachments; an awake agent reads it and may answer. */
export function addComment(ctx: Ctx, cid: CardKey, text: string, att?: Attachment[]): void {
  const c = card(ctx, cid);
  if (!c || (!text.trim() && !att?.length)) return;
  const cm = live<Comment>({
    id: `co${takeCk(ctx.ids)}`,
    author: ME,
    text: text.trim(),
    ts: Date.now(),
    att: [...(att || []), ...linksIn(text)],
    read: false,
  });
  c.comments.push(cm);
  addAct(ctx, cid, { kind: "tool", text: "You commented" });
  toast(ctx, "Comment added");
  if (!isAwake(c)) return;
  later(READ_MS, () => {
    cm.read = true;
    addAct(ctx, cid, { kind: "tool", text: `${c.agent} read your comment` });
    if (asksAgent(text)) later(ANSWER_MS, () => agentAnswer(ctx, cid, cm));
  });
}

export function deleteComment(ctx: Ctx, cid: CardKey, id: string): void {
  const c = card(ctx, cid);
  if (!c) return;
  c.comments = c.comments.filter((x) => x.id !== id);
  toast(ctx, "Comment deleted");
}

export function toggleMember(ctx: Ctx, cid: CardKey, pid: string): void {
  const c = card(ctx, cid);
  if (!c) return;
  c.members = c.members.includes(pid) ? c.members.filter((x) => x !== pid) : [...c.members, pid];
  const verb = c.members.includes(pid) ? "Added " : "Removed ";
  addAct(ctx, cid, { kind: "tool", text: `${verb}${person(ctx, pid)?.name}` });
}
