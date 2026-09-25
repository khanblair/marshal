import { batch } from "solid-js";
import { type CardKey, cardLabel } from "../card-key";
import type { Ctx } from "../context";
import { confirm, later, live, stream, toast } from "../engine";
import { takeMid } from "../ids";
import { chatById, needs, working } from "../selectors";
import type { Chat, Msg } from "../types";
import { insertCard, newCardFrom } from "./card-create";

/** Delay before the project agent answers. */
const REPLY_MS = 500;
/** A fresh chat is named after the first words of its first message. */
const TITLE_WORDS = 6;
const MERGE_START_PCT = 10;

const upperFirst = (s: string): string => s.charAt(0).toUpperCase() + s.slice(1);
const lowerFirst = (s: string): string => s.charAt(0).toLowerCase() + s.slice(1);

function replyBlocked(ctx: Ctx, pid: string, list: Msg[]): void {
  const n = needs(ctx, pid);
  const text = n.length
    ? `${n.length}${n.length === 1 ? " card is" : " cards are"} waiting on you: ${n
        .map((c) => `${cardLabel(c)} ${lowerFirst(c.reason)}`)
        .join(". ")}.`
    : "Nothing is blocked right now.";
  list.push(
    ctx.msg.links(
      text,
      n.map((c) => c.id),
    ),
  );
}

/** Three backlog cards for `topic`; each depends on the one before. */
const PLAN_ROWS = [
  { title: (t: string) => `Design the ${t} data model`, role: "Worker" },
  { title: (t: string) => `Build the ${t} API`, role: "Worker" },
  { title: (t: string) => `Test the ${t} flow`, role: "Tester" },
];
const PLANNED_SPAN = { s: 1, e: 4 };

function replyMakeCards(ctx: Ctx, pid: string, list: Msg[], text: string): void {
  const topic = text.match(/for (the )?(.+)$/i)?.[2] || "this work";
  stream(
    ctx,
    list,
    "I split it into three cards. They start in Backlog, and the second depends on the first.",
    () => {
      let prev: CardKey | null = null;
      for (const row of PLAN_ROWS) {
        const c = insertCard(
          ctx,
          newCardFrom(ctx, {
            p: pid,
            title: upperFirst(row.title(topic)),
            state: "backlog",
            role: row.role,
            ...PLANNED_SPAN,
            deps: prev ? [prev] : [],
            upd: 0,
          }),
        );
        prev = c.id;
        list.push(ctx.msg.cardRef(c.id));
      }
    },
  );
}

function replyMerge(ctx: Ctx, pid: string, list: Msg[]): void {
  const r = ctx.S.cards.find((c) => c.p === pid && c.state === "ready");
  if (!r) {
    stream(ctx, list, "No cards are ready to merge in this project.");
    return;
  }
  r.state = "merging";
  r.mergePct = MERGE_START_PCT;
  r.doing = "Dry-run merge with git merge-tree";
  stream(
    ctx,
    list,
    `${cardLabel(r)} entered the merge queue. The Integrator is running a dry-run merge first.`,
  );
  list.push(ctx.msg.cardRef(r.id));
}

function replyStatus(ctx: Ctx, pid: string, list: Msg[], who: string): void {
  const w = working(ctx, pid).length;
  const n = needs(ctx, pid).length;
  const intro = who === "Orchestrator" ? "" : `${who} here. `;
  stream(
    ctx,
    list,
    `${intro}${w} cards are working and ${n} need you. I can make cards for this, or send it to a card. Try "What is blocked?" or "Make cards for the export work".`,
  );
}

function reply(ctx: Ctx, ch: Chat, text: string): void {
  const q = text.toLowerCase();
  const list = ch.msgs;
  if (/block|stuck|waiting/.test(q)) replyBlocked(ctx, ch.pid, list);
  else if (/(make|create|add).*(card|task)/.test(q)) replyMakeCards(ctx, ch.pid, list, text);
  else if (/merge/.test(q)) replyMerge(ctx, ch.pid, list);
  else replyStatus(ctx, ch.pid, list, ch.target);
}

/** Sends a message in a project chat; the agent answers with a scripted reply. */
export function chatSend(ctx: Ctx, pid: string, cid: string, text: string): void {
  if (!text.trim()) return;
  const ch = chatById(ctx, pid, cid);
  if (!ch) return;
  if (ch.fresh) {
    const words = text
      .replace(/[?.!]+$/, "")
      .split(/\s+/)
      .slice(0, TITLE_WORDS)
      .join(" ");
    ch.title = upperFirst(words);
    ch.fresh = false;
  }
  ch.last = Date.now();
  ch.msgs.push(ctx.msg.user(text));
  later(REPLY_MS, () => reply(ctx, ch, text));
}

export function openChat(ctx: Ctx, pid: string, id: string | null): void {
  ctx.S.chatOpen[pid] = id;
}

export function newChat(ctx: Ctx, pid: string, target?: string): Chat {
  const { S } = ctx;
  const chat = live<Chat>({
    id: `ch${takeMid(ctx.ids)}`,
    pid,
    title: "New chat",
    fresh: true,
    target: target || "Orchestrator",
    msgs: [],
    last: Date.now(),
    archived: false,
  });
  const list = S.chats[pid];
  if (list) list.push(chat);
  else S.chats[pid] = [chat];
  S.chatOpen[pid] = chat.id;
  return chat;
}

export function renameChat(ctx: Ctx, pid: string, id: string, title: string): boolean {
  const c = chatById(ctx, pid, id);
  if (!title?.trim()) {
    toast(ctx, "Chat names can't be empty. The old name is kept.");
    return false;
  }
  if (!c) return false;
  c.title = title.trim();
  c.fresh = false;
  return true;
}

export function archiveChat(ctx: Ctx, pid: string, id: string, on: boolean): void {
  const c = chatById(ctx, pid, id);
  if (!c) return;
  c.archived = on;
  if (on && ctx.S.chatOpen[pid] === id) ctx.S.chatOpen[pid] = null;
  if (!on) {
    toast(ctx, "Chat restored");
    return;
  }
  toast(ctx, "Chat archived", {
    label: "Undo",
    run: () => batch(() => archiveChat(ctx, pid, id, false)),
  });
}

export function deleteChat(ctx: Ctx, pid: string, id: string): void {
  const { S } = ctx;
  const c = chatById(ctx, pid, id);
  if (!c) return;
  confirm(ctx, {
    title: "Delete chat",
    message: `This deletes "${c.title}" and its messages. Cards it created stay on the board.`,
    action: "Delete chat",
    destructive: true,
    run: () =>
      batch(() => {
        S.chats[pid] = (S.chats[pid] ?? []).filter((x) => x !== c);
        if (S.chatOpen[pid] === id) S.chatOpen[pid] = null;
        toast(ctx, "Chat deleted");
      }),
  });
}
