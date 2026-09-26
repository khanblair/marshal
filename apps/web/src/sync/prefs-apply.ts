import { batch } from "solid-js";
import type { Ctx } from "~/mock/context";
import type { State } from "~/mock/state-types";
import type { Filter, SortSpec, SwimKey, Theme, ViewKey } from "~/mock/types";
import { ALL_CARDS, toLaneKey } from "./me-mapper";
import { PROJECT_FIELDS, type ProjectField, savedViewIdOf } from "./prefs-model";

/** One preference to write into the store: its key (see `prefs-model.ts`) and its value. */
export type PrefWrite = readonly [key: string, value: unknown];

/** Folds exactly the lanes the daemon lists for the project and unfolds the rest. */
function writeLanes(S: State, pid: string, lanes: readonly string[]): void {
  const wanted = new Set(lanes.map((lane) => toLaneKey(pid, lane)));
  for (const key of Object.keys(S.laneCollapsed)) {
    if (key.startsWith(`${pid}:`) && !wanted.has(key)) delete S.laneCollapsed[key];
  }
  for (const key of wanted) if (!S.laneCollapsed[key]) S.laneCollapsed[key] = true;
}

function writeProjectField(S: State, pid: string, field: ProjectField, value: unknown): void {
  switch (field) {
    case "lastView":
      S.lastView[pid] = value as ViewKey;
      break;
    case "filters":
      S.filters[pid] = (value as Filter[]).map((filter) => ({ k: filter.k, v: filter.v }));
      break;
    case "query":
      S.query[pid] = value as string;
      break;
    case "swim":
      S.swim[pid] = value as SwimKey;
      break;
    case "lanes":
      writeLanes(S, pid, value as string[]);
      break;
    case "showAllDone":
      S.showAllDone[pid] = value as boolean;
      break;
    case "savedViewId":
      // The store names the view in use, not its id, so this is settled by `settleViewName`.
      break;
  }
}

/**
 * The name the menu shows for the saved view in use. A view the daemon has by id shows by its name.
 * With none in use the menu says "All cards" only when nothing is filtered or grouped, which is
 * what that view is; anything else was set by hand.
 */
function settleViewName(S: State, pid: string, id: string | null): void {
  let name: string | null;
  if (id) name = S.savedViews[pid]?.find((view) => view.id === id)?.name ?? null;
  else name = !S.filters[pid]?.length && S.swim[pid] === "none" ? ALL_CARDS : null;
  if (S.savedView[pid] !== name) S.savedView[pid] = name;
}

const isProjectField = (name: string): name is ProjectField =>
  (PROJECT_FIELDS as readonly string[]).includes(name);

/** The projects whose saved view in use must be worked out again after a write, and the id the daemon named for it, if it did. */
type ViewsToSettle = Map<string, string | null | undefined>;

/** Writes one project's field, and notes when it changes what the saved view in use means. */
function writeProjectKey(S: State, key: string, value: unknown, settle: ViewsToSettle): void {
  const at = key.indexOf(":");
  const pid = key.slice(0, at);
  const field = key.slice(at + 1);
  if (!isProjectField(field)) return;
  writeProjectField(S, pid, field, value);
  if (field === "savedViewId") settle.set(pid, value as string | null);
  // Filters and the swimlane change what the view in use is, so its name is worked out again.
  else if ((field === "filters" || field === "swim") && !settle.has(pid))
    settle.set(pid, undefined);
}

/** Writes one key that is not a project's: the theme, a List column, or a table's sort. */
function writeOwnKey(S: State, key: string, value: unknown): void {
  if (key === "#theme") S.theme = value as Theme;
  else if (key.startsWith("#col:")) S.listCols[key.slice("#col:".length)] = value as boolean;
  else {
    const table = key.slice("#sort:".length) as "agents" | "list";
    S.sort[table] = { ...(value as SortSpec) };
  }
}

/**
 * Writes preferences into the store in one batch, and applies the theme to the page if it changed.
 * Only what the caller found different is passed, so nothing redraws for a value the store has.
 */
export function writePrefs(ctx: Ctx, writes: readonly PrefWrite[]): void {
  const { S } = ctx;
  const settle: ViewsToSettle = new Map();
  batch(() => {
    for (const [key, value] of writes) {
      if (key.startsWith("#")) writeOwnKey(S, key, value);
      else writeProjectKey(S, key, value, settle);
    }
    for (const [pid, id] of settle) {
      settleViewName(S, pid, id === undefined ? savedViewIdOf(S, pid) : id);
    }
  });
  if (writes.some(([key]) => key === "#theme")) ctx.env.applyTheme(S);
}
