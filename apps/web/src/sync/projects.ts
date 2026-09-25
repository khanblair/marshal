import {
  EventTypeProjectCreated,
  EventTypeProjectRemoved,
  EventTypeProjectUpdated,
  type Event as WireEvent,
  type Project as WireProject,
} from "@marshal/protocol";
import { batch } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import { type DaemonProject, toDaemonProject } from "~/data/mappers/project";
import type { Ctx } from "~/mock/context";
import { feed } from "~/mock/engine";
import type { State } from "~/mock/state-types";
import type { Project } from "~/mock/types";
import { dropMock, reconcileMock } from "./reservoir";
import type { Syncer } from "./syncer";

/** The limits a project starts with, as the mock's own "New project" gave them. */
const NEW_PROJECT_LIMITS = { day: 8, month: 120, awake: 6 };

/** The fields the daemon fills. The mock-only ones (`ci`, `ciAgo`, `monthBase`, `runs`) are never touched. */
type Mirrored = Pick<
  Project,
  "id" | "name" | "lang" | "path" | "packages" | "branch" | "dev" | "lockBypass"
>;

function mirrored(project: DaemonProject): Mirrored {
  return {
    id: project.id,
    name: project.name,
    lang: project.lang,
    path: project.path,
    packages: project.packages,
    branch: project.branch,
    dev: project.dev,
    lockBypass: project.lockBypass,
  };
}

const sameList = (a: readonly string[] | undefined, b: readonly string[] | undefined): boolean =>
  a === b || (!!a && !!b && a.length === b.length && a.every((item, i) => item === b[i]));

/** Writes the daemon's values into a project that is already in the store, one field at a time, so nothing redraws that did not change. */
function update(target: Project, next: Mirrored): void {
  if (target.name !== next.name) target.name = next.name;
  if (target.lang !== next.lang) target.lang = next.lang;
  if (target.path !== next.path) target.path = next.path;
  if (target.branch !== next.branch) target.branch = next.branch;
  if (target.dev !== next.dev) target.dev = next.dev;
  if (target.lockBypass !== next.lockBypass) target.lockBypass = next.lockBypass;
  if (!sameList(target.packages, next.packages)) target.packages = next.packages;
}

/**
 * Screen state every project has: its filters, search, swimlane, saved views, last view, chats, and
 * limits. It is filled for a project that has none, once, whether the project was made here, on
 * another device, or by the fixture, and it never overwrites what a project already has.
 */
export function ensureProjectState(S: State, project: Pick<Project, "id" | "packages">): void {
  const { id } = project;
  if (!(id in S.filters)) S.filters[id] = [];
  if (!(id in S.query)) S.query[id] = "";
  if (!(id in S.swim)) S.swim[id] = project.packages ? "package" : "none";
  if (!(id in S.savedViews)) S.savedViews[id] = [{ name: "All cards", f: [], swim: "none" }];
  if (!(id in S.savedView)) S.savedView[id] = "All cards";
  if (!(id in S.lastView)) S.lastView[id] = "board";
  if (!(id in S.chats)) S.chats[id] = [];
  if (!(id in S.limits)) S.limits[id] = { ...NEW_PROJECT_LIMITS };
}

/** The project shown on Home and in the settings must be one that exists. */
function repairRoute(S: State): void {
  if (S.route.pid && S.projects.some((project) => project.id === S.route.pid)) return;
  S.route.pid = S.projects[0]?.id ?? null;
}

/** After the project list changed: its mock records, its per-project state, and where the screen points. */
function settle(ctx: Ctx): void {
  reconcileMock(ctx);
  for (const project of ctx.S.projects) ensureProjectState(ctx.S, project);
  repairRoute(ctx.S);
}

/**
 * A project that is gone: its mock records, the open card if it was one of its, and one line in the
 * feed. Its screen state (filters, views, limits) stays, as it always did, so a project added again
 * under the same id finds it as it was. The repository on disk is not touched, and the line says so.
 */
export function forgetProject(ctx: Ctx, project: Pick<Project, "id" | "name">): void {
  const { S } = ctx;
  batch(() => {
    S.projects = S.projects.filter((p) => p.id !== project.id);
    dropMock(ctx, project.id);
    if (S.route.pid === project.id) {
      S.route = { page: "home", pid: S.projects[0]?.id ?? null, view: "board" };
    }
    settle(ctx);
    feed(ctx, {
      kind: "tool",
      text: `${project.name} was removed from Marshal. The repository on disk was not touched.`,
      pid: null,
    });
  });
}

const sameOrder = (a: readonly Project[], b: readonly Project[]): boolean =>
  a.length === b.length && a.every((project, i) => project === b[i]);

/**
 * Makes the store's project list the daemon's: each project is updated in place (a row that did not
 * change is not redrawn), new ones are added, the ones the daemon no longer has are forgotten, and
 * the order is the daemon's. Applying the same list twice changes nothing.
 */
export function applyProjectSnapshot(ctx: Ctx, projects: readonly DaemonProject[]): void {
  const { S } = ctx;
  batch(() => {
    const known = new Map(S.projects.map((project) => [project.id, project]));
    const wanted = new Set(projects.map((project) => project.id));
    const gone = S.projects.filter((project) => !wanted.has(project.id));
    const next = projects.map((project) => {
      const existing = known.get(project.id);
      if (existing) update(existing, mirrored(project));
      return existing ?? mirrored(project);
    });
    if (!sameOrder(S.projects, next)) S.projects = next;
    for (const project of gone) forgetProject(ctx, project);
    settle(ctx);
  });
}

/** One project created or changed. It goes where it is in the list, or at the end when it is new. */
export function applyProject(ctx: Ctx, project: DaemonProject): void {
  const { S } = ctx;
  batch(() => {
    const existing = S.projects.find((p) => p.id === project.id);
    if (existing) update(existing, mirrored(project));
    else S.projects.push(mirrored(project));
    settle(ctx);
  });
}

/** A project removed on the daemon, by id. It does nothing when the store already forgot it. */
export function applyProjectRemoved(ctx: Ctx, id: string): void {
  const project = ctx.S.projects.find((p) => p.id === id);
  if (project) forgetProject(ctx, project);
  else dropMock(ctx, id);
}

function readWireProject(data: unknown): WireProject | null {
  if (!isRecord(data) || !isRecord(data.project) || typeof data.project.id !== "string")
    return null;
  return data.project as unknown as WireProject;
}

/** The `project.*` events of the Home topic. Each carries the project as it is now, so applying one twice is harmless. */
function applyProjectEvent(ctx: Ctx, event: WireEvent): void {
  if (event.type === EventTypeProjectRemoved) {
    if (isRecord(event.data) && typeof event.data.projectId === "string") {
      applyProjectRemoved(ctx, event.data.projectId);
    }
    return;
  }
  if (event.type !== EventTypeProjectCreated && event.type !== EventTypeProjectUpdated) return;
  const project = readWireProject(event.data);
  if (project) applyProject(ctx, toDaemonProject(project));
}

/** Section S3: the project list, its `project.*` events, and nothing else. */
export const projectsSyncer: Syncer<DaemonProject[]> = {
  section: "S3",
  topics: ["home"],
  async load(api: ApiClient) {
    const list = await api.listProjects();
    return list.projects.map(toDaemonProject);
  },
  apply: applyProjectSnapshot,
  onEvent: applyProjectEvent,
};
