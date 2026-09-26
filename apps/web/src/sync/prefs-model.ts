import type {
  Preferences,
  UpdatePreferencesRequest,
  UpdateProjectPreferences,
} from "@marshal/protocol";
import { DEFAULT_LIST_COLS, DEFAULT_SORT } from "~/mock/state";
import type { State } from "~/mock/state-types";
import type { Filter, Project, SortSpec, SwimKey, Theme, ViewKey } from "~/mock/types";
import {
  isAllCards,
  toSortSpec,
  toStoredFilters,
  toWireFilters,
  toWireLanes,
  toWireSort,
} from "./me-mapper";

/*
 * The screen preferences as one flat map, so a change is a difference between two maps.
 *
 * The daemon keeps the theme, the List's columns, both tables' sort, and for each project its last
 * view, filters, search, swimlane, folded lanes, and saved view (decision D2). The screens write all
 * of these straight into `M.S` from many places, so nothing here waits to be told about a change:
 * the map read from the store is compared with the map the daemon last agreed to (`base`), and what
 * differs is what is sent. The same comparison, with the daemon's newest answer as a third side, says
 * which of the daemon's values the store may take: a value the person changed and the daemon has
 * not seen yet is kept.
 *
 * Keys: `#theme`, `#col:<column>`, `#sort:agents`, `#sort:list`, and `<project>:<field>` for the
 * project fields. A project id is a lower case slug, so it never starts with `#` or holds a colon.
 */

/** Every preference by its key. A value is plain data, so two are the same when their JSON is. */
export type PrefMap = Map<string, unknown>;

export const PROJECT_FIELDS = [
  "lastView",
  "filters",
  "query",
  "swim",
  "lanes",
  "showAllDone",
  "savedViewId",
] as const;
export type ProjectField = (typeof PROJECT_FIELDS)[number];

export const projectKey = (pid: string, field: ProjectField): string => `${pid}:${field}`;

export const samePref = (a: unknown, b: unknown): boolean =>
  JSON.stringify(a) === JSON.stringify(b);

/**
 * The id of the saved view in use, from its name. The client's own "All cards" has none, and neither
 * does a daemon view that is the same thing (see `isAllCards`): using it is using none.
 */
export function savedViewIdOf(S: State, pid: string): string | null {
  const name = S.savedView[pid];
  const view = name ? S.savedViews[pid]?.find((one) => one.name === name) : undefined;
  return view?.id && !isAllCards(view) ? view.id : null;
}

/**
 * A project's preferences as the daemon has them when it has saved nothing for it: its own defaults.
 * These are not what the client starts a project with (`ensureProjectState` puts a monorepo on the
 * package swimlane, and the daemon's default is none), and that difference is meant: it is what is
 * sent the first time, so the row the daemon then makes holds the swimlane the person sees. Were
 * `base` the client's defaults, the first other change to the project would make the daemon's row
 * with its own swimlane, and the answer would take the package swimlane away.
 */
function defaultEntries(): [ProjectField, unknown][] {
  return [
    ["lastView", "board"],
    ["filters", []],
    ["query", ""],
    ["swim", "none"],
    ["lanes", []],
    ["showAllDone", false],
    ["savedViewId", null],
  ];
}

function projectEntries(S: State, project: Project): [ProjectField, unknown][] {
  const { id } = project;
  return [
    ["lastView", S.lastView[id] ?? "board"],
    ["filters", (S.filters[id] ?? []).map((filter) => ({ k: filter.k, v: filter.v }))],
    ["query", S.query[id] ?? ""],
    ["swim", S.swim[id] ?? (project.packages ? "package" : "none")],
    ["lanes", toWireLanes(id, S.laneCollapsed)],
    ["showAllDone", !!S.showAllDone[id]],
    ["savedViewId", savedViewIdOf(S, id)],
  ];
}

/** The preferences as the store holds them now. Read inside an effect, it follows every one of them. */
export function readPrefs(S: State): PrefMap {
  const map: PrefMap = new Map();
  map.set("#theme", S.theme);
  for (const [column, on] of Object.entries(S.listCols)) map.set(`#col:${column}`, !!on);
  map.set("#sort:agents", { k: S.sort.agents.k, dir: S.sort.agents.dir });
  map.set("#sort:list", { k: S.sort.list.k, dir: S.sort.list.dir });
  for (const project of S.projects) {
    for (const [field, value] of projectEntries(S, project)) {
      map.set(projectKey(project.id, field), value);
    }
  }
  return map;
}

/** What the daemon has for a person who changed nothing, for the projects the store knows (see `defaultEntries`). */
export function defaultBase(S: State): PrefMap {
  const map: PrefMap = new Map([["#theme", "system"]]);
  for (const [column, on] of Object.entries(DEFAULT_LIST_COLS)) map.set(`#col:${column}`, on);
  map.set("#sort:agents", { ...DEFAULT_SORT.agents });
  map.set("#sort:list", { ...DEFAULT_SORT.list });
  completeBase(map, S.projects);
  return map;
}

/** Gives a project that arrived after the last answer the defaults the daemon has for it. */
export function completeBase(base: PrefMap, projects: readonly Project[]): void {
  for (const project of projects) {
    for (const [field, value] of defaultEntries()) {
      const key = projectKey(project.id, field);
      if (!base.has(key)) base.set(key, value);
    }
  }
}

/** Forgets a project's keys, as the daemon does when the project is removed. */
export function dropProjectKeys(base: PrefMap, pid: string): void {
  for (const key of [...base.keys()]) if (key.startsWith(`${pid}:`)) base.delete(key);
}

/**
 * What the daemon's preferences say, key by key. A value it has no opinion on is left out: a table
 * that was never sorted, a column that was never chosen, and a project it saved nothing for.
 * The daemon names the view in use by id, so an id that is not in the project's list (a view that
 * was deleted) is read as none.
 */
export function prefsFromWire(wire: Preferences, S: State): PrefMap {
  const map: PrefMap = new Map([["#theme", wire.theme]]);
  for (const [column, on] of Object.entries(wire.listColumns)) map.set(`#col:${column}`, on);
  for (const table of ["agents", "list"] as const) {
    const order = wire.sort[table];
    if (order) map.set(`#sort:${table}`, toSortSpec(order, DEFAULT_SORT[table]));
  }
  for (const [pid, prefs] of Object.entries(wire.projects)) {
    // A project the store does not have is not drawn, so its preferences wait until it arrives.
    if (!S.projects.some((project) => project.id === pid)) continue;
    const id = prefs.savedViewId ?? null;
    map.set(projectKey(pid, "lastView"), prefs.lastView);
    map.set(projectKey(pid, "filters"), toStoredFilters(prefs.filters));
    map.set(projectKey(pid, "query"), prefs.query);
    map.set(projectKey(pid, "swim"), prefs.swimlane);
    map.set(projectKey(pid, "lanes"), [...prefs.collapsedLanes].sort());
    map.set(projectKey(pid, "showAllDone"), prefs.showAllDone);
    const named = id ? S.savedViews[pid]?.find((view) => view.id === id) : undefined;
    map.set(projectKey(pid, "savedViewId"), named && !isAllCards(named) ? id : null);
  }
  return map;
}

/** The keys whose value in `now` is not the one in `base`. */
export function changedKeys(base: PrefMap, now: PrefMap): string[] {
  return [...now.keys()].filter((key) => !samePref(base.get(key), now.get(key)));
}

/** The fields of one project that a key names, as the wire's own words for them. */
function setProjectField(change: UpdateProjectPreferences, field: string, value: unknown): void {
  switch (field) {
    case "lastView":
      change.lastView = value as ViewKey;
      break;
    case "filters":
      change.filters = toWireFilters(value as Filter[]);
      break;
    case "query":
      change.query = value as string;
      break;
    case "swim":
      change.swimlane = value as SwimKey;
      break;
    case "lanes":
      change.collapsedLanes = value as string[];
      break;
    case "showAllDone":
      change.showAllDone = value as boolean;
      break;
    case "savedViewId":
      // The empty string is how the daemon is told the view in use was let go.
      change.savedViewId = (value as string | null) ?? "";
      break;
  }
}

/** The request that says what the given keys are now. Only those keys are in it, so nothing else is reset. */
export function toRequest(now: PrefMap, keys: readonly string[]): UpdatePreferencesRequest {
  const request: UpdatePreferencesRequest = {};
  for (const key of keys) {
    const value = now.get(key);
    if (key === "#theme") request.theme = value as Theme;
    else if (key.startsWith("#col:")) {
      request.listColumns = { ...request.listColumns, [key.slice("#col:".length)]: !!value };
    } else if (key.startsWith("#sort:")) {
      const table = key.slice("#sort:".length) as "agents" | "list";
      request.sort = { ...request.sort, [table]: toWireSort(value as SortSpec) };
    } else {
      const at = key.indexOf(":");
      const pid = key.slice(0, at);
      const change = request.projects?.[pid] ?? {};
      setProjectField(change, key.slice(at + 1), value);
      request.projects = { ...request.projects, [pid]: change };
    }
  }
  return request;
}
