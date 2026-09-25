import { colOf } from "../constants";
import type { Ctx } from "../context";
import { toast } from "../engine";
import { cardsOf } from "../selectors";
import type { Card, Filter, FilterKey } from "../types";

type FilterValues = Partial<Record<FilterKey, string[]>>;

function groupFilters(filters: readonly Filter[]): FilterValues {
  const by: FilterValues = {};
  for (const f of filters) by[f.k] = [...(by[f.k] ?? []), f.v];
  return by;
}

/** Filters of different kinds must all match; values of one kind match any. */
function matchesFilters(c: Card, by: FilterValues): boolean {
  if (by.status && !by.status.includes(colOf(c.state))) return false;
  if (by.role && !by.role.includes(c.role)) return false;
  if (by.agent && !by.agent.includes(c.agent)) return false;
  if (by.model && !by.model.includes(c.model)) return false;
  if (by.label && !by.label.some((l) => c.labels.includes(l))) return false;
  return !by.package || by.package.includes(c.pkg || "No package");
}

const matchesQuery = (c: Card, q: string): boolean =>
  !q || c.title.toLowerCase().includes(q) || `#${c.id}`.includes(q) || (c.branch || "").includes(q);

/** The project's cards after its filter chips and search text. */
export function filtered(ctx: Ctx, pid: string): Card[] {
  const by = groupFilters(ctx.S.filters[pid] ?? []);
  const q = (ctx.S.query[pid] || "").toLowerCase();
  return cardsOf(ctx, pid).filter((c) => matchesFilters(c, by) && matchesQuery(c, q));
}

/* The actions below work on the current project. Changing filters by hand unselects the saved view. */

export function addFilter(ctx: Ctx, k: FilterKey, v: string): void {
  const { S } = ctx;
  const pid = S.route.pid;
  if (!pid) return;
  const f = S.filters[pid] ?? [];
  if (!f.find((x) => x.k === k && x.v === v)) S.filters[pid] = [...f, { k, v }];
  S.savedView[pid] = null;
  S.menu = null;
}

export function removeFilter(ctx: Ctx, k: FilterKey, v: string): void {
  const { S } = ctx;
  const pid = S.route.pid;
  if (!pid) return;
  S.filters[pid] = (S.filters[pid] ?? []).filter((x) => !(x.k === k && x.v === v));
  S.savedView[pid] = null;
}

export function clearFilters(ctx: Ctx): void {
  const { S } = ctx;
  const pid = S.route.pid;
  if (!pid) return;
  S.filters[pid] = [];
  S.query[pid] = "";
}

export function applyView(ctx: Ctx, name: string): void {
  const { S } = ctx;
  const pid = S.route.pid;
  const v = pid ? (S.savedViews[pid] ?? []).find((x) => x.name === name) : undefined;
  if (!pid || !v) return;
  S.filters[pid] = v.f.slice();
  S.swim[pid] = v.swim;
  S.savedView[pid] = name;
  S.menu = null;
}

export function saveView(ctx: Ctx, name: string): void {
  const { S } = ctx;
  const pid = S.route.pid;
  if (!pid || !name) return;
  const others = (S.savedViews[pid] ?? []).filter((v) => v.name !== name);
  S.savedViews[pid] = [
    ...others,
    { name, f: (S.filters[pid] ?? []).slice(), swim: S.swim[pid] ?? "none" },
  ];
  S.savedView[pid] = name;
  S.menu = null;
  toast(ctx, "View saved");
}
