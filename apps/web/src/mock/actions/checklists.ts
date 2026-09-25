import { batch } from "solid-js";
import type { Ctx } from "../context";
import { addAct, confirm, toast } from "../engine";
import { takeCk } from "../ids";
import { card } from "../selectors";
import type { Checklist } from "../types";

/** The signed-in user; checked items record who checked them. */
const ME = "ada";

const listOf = (ctx: Ctx, cid: number, lid: string): Checklist | undefined =>
  card(ctx, cid)?.checklists.find((x) => x.id === lid);

export function toggleItem(ctx: Ctx, cid: number, lid: string, iid: string): void {
  const it = listOf(ctx, cid, lid)?.items.find((x) => x.id === iid);
  if (!it) return;
  it.done = !it.done;
  it.by = it.done ? ME : null;
  it.doneAt = Date.now();
  addAct(ctx, cid, {
    kind: "tool",
    text: `${it.done ? "You completed " : "You reopened "}${it.text}`,
  });
}

export function addItem(ctx: Ctx, cid: number, lid: string, text: string): void {
  if (!text?.trim()) return;
  const l = listOf(ctx, cid, lid);
  if (!l) return;
  l.items.push({ id: `it${takeCk(ctx.ids)}`, text: text.trim(), done: false, by: null });
}

export function removeItem(ctx: Ctx, cid: number, lid: string, iid: string): void {
  const l = listOf(ctx, cid, lid);
  if (l) l.items = l.items.filter((x) => x.id !== iid);
}

export function addChecklist(ctx: Ctx, cid: number, title?: string): void {
  const c = card(ctx, cid);
  if (!c) return;
  c.checklists.push({
    id: `cl${takeCk(ctx.ids)}`,
    title: (title || "").trim() || "Checklist",
    hideDone: false,
    items: [],
  });
  toast(ctx, "Checklist added");
}

export function deleteChecklist(ctx: Ctx, cid: number, lid: string): void {
  const c = card(ctx, cid);
  const l = listOf(ctx, cid, lid);
  if (!c || !l) return;
  confirm(ctx, {
    title: "Delete checklist",
    message: `This deletes "${l.title}" and its ${l.items.length} items.`,
    action: "Delete checklist",
    destructive: true,
    run: () =>
      batch(() => {
        c.checklists = c.checklists.filter((x) => x !== l);
        toast(ctx, "Checklist deleted");
      }),
  });
}

export function toggleHideDone(ctx: Ctx, cid: number, lid: string): void {
  const l = listOf(ctx, cid, lid);
  if (l) l.hideDone = !l.hideDone;
}
