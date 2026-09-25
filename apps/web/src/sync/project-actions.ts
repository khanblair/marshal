import type { CreateProjectRequest, UpdateProjectRequest } from "@marshal/protocol";
import { batch } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { ApiError } from "~/data/api-error";
import { toDaemonProject } from "~/data/mappers/project";
import { ChangeInFlightError } from "~/data/optimistic";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";
import { proj } from "~/mock/selectors";
import type { Project } from "~/mock/types";
import { applyProject, forgetProject } from "./projects";
import { reconcileMock } from "./reservoir";

const NOT_CONNECTED = "Marshal is not connected to its daemon.";
const GENERIC_FAILURE = "Marshal could not finish that. Try again.";

/** What the New project dialog needs back: the new project's id, or the daemon's sentence to show. */
export type AddProjectResult = { id: string } | { error: string };

/** The fields the project settings save. `branch` and `dev` are the mock's names for the daemon's. */
export interface ProjectSettings {
  name: string;
  branch: string;
  dev: string;
  lockBypass: boolean;
}

function apiOf(ctx: Ctx): ApiClient | null {
  return ctx.env.data?.api ?? null;
}

const sentence = (error: unknown): string =>
  error instanceof ApiError ? error.message : GENERIC_FAILURE;

/**
 * Runs a change through `optimistic`, which shows the daemon's refusal itself. What it stays silent
 * about is a second change to the same project while one is still being saved, so that is said here.
 */
async function attempt(ctx: Ctx, run: (api: ApiClient) => Promise<void>): Promise<boolean> {
  const api = apiOf(ctx);
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return false;
  }
  try {
    await run(api);
    return true;
  } catch (error) {
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
    return false;
  }
}

/**
 * Adds a project on the daemon, which finds its language and packages itself. There is nothing to
 * draw before it answers, so nothing is guessed: on success the answer goes in through the same
 * function the `project.created` event uses, so the event that follows changes nothing. The daemon's
 * refusal comes back as a sentence for the dialog to show, and nothing is toasted.
 */
export async function addProject(
  ctx: Ctx,
  request: CreateProjectRequest,
): Promise<AddProjectResult> {
  const api = apiOf(ctx);
  if (!api) return { error: NOT_CONNECTED };
  try {
    const created = toDaemonProject(await api.createProject(request));
    applyProject(ctx, created);
    return { id: created.id };
  } catch (error) {
    return { error: sentence(error) };
  }
}

/** Renames a project. Returns false, with the old name kept, when the name is empty or the daemon refuses. */
export async function renameProject(ctx: Ctx, id: string, name: string): Promise<boolean> {
  const project = proj(ctx, id);
  const next = name.trim();
  if (!next) {
    toast(ctx, "Project names can't be empty. The old name is kept.");
    return false;
  }
  if (!project) return false;
  if (next === project.name) return true;
  const old = project.name;
  return attempt(ctx, async (api) => {
    const updated = await ctx.optimistic({
      key: `update:${id}`,
      apply: () => {
        project.name = next;
      },
      request: () => api.updateProject(id, { name: next }),
      rollback: () => {
        project.name = old;
      },
    });
    applyProject(ctx, toDaemonProject(updated));
  });
}

/** Only what changed goes to the daemon, so a save cannot undo another device's edit of another field. */
function changesOf(project: Project, next: ProjectSettings): UpdateProjectRequest {
  const body: UpdateProjectRequest = {};
  if (next.name !== project.name) body.name = next.name;
  if (next.branch !== (project.branch ?? "")) body.defaultBranch = next.branch;
  if (next.dev !== (project.dev ?? "")) body.devCommand = next.dev;
  if (next.lockBypass !== !!project.lockBypass) body.bypassLocked = next.lockBypass;
  return body;
}

/** Saves the project settings form. Returns true when nothing needed saving or the daemon accepted it. */
export async function saveProject(ctx: Ctx, id: string, next: ProjectSettings): Promise<boolean> {
  const project = proj(ctx, id);
  if (!project) return false;
  const body = changesOf(project, next);
  if (!Object.keys(body).length) return true;
  const old: ProjectSettings = {
    name: project.name,
    branch: project.branch ?? "",
    dev: project.dev ?? "",
    lockBypass: !!project.lockBypass,
  };
  const set = (values: ProjectSettings) => () => {
    batch(() => Object.assign(project, values));
  };
  return attempt(ctx, async (api) => {
    const updated = await ctx.optimistic({
      key: `update:${id}`,
      apply: set(next),
      request: () => api.updateProject(id, body),
      rollback: set(old),
    });
    applyProject(ctx, toDaemonProject(updated));
    toast(ctx, "Project saved");
  });
}

/**
 * Removes a project from Marshal. The two choices always go to the daemon as they are, because a
 * request with no body means "keep nothing". The project leaves the list at once (with its mock
 * records) and comes back if the daemon refuses. The repository folder is never touched.
 */
export async function removeProject(
  ctx: Ctx,
  id: string,
  keep: { keepBranches: boolean; keepMemory: boolean },
): Promise<boolean> {
  const project = proj(ctx, id);
  if (!project) return false;
  const before = [...ctx.S.projects];
  const done = await attempt(ctx, async (api) => {
    await ctx.optimistic({
      key: `remove:${id}`,
      apply: () =>
        batch(() => {
          ctx.S.projects = before.filter((p) => p.id !== id);
          reconcileMock(ctx);
          if (ctx.S.route.pid === id) {
            ctx.S.route = { page: "home", pid: ctx.S.projects[0]?.id ?? null, view: "board" };
          }
        }),
      request: () =>
        api.removeProject(id, { keepBranches: keep.keepBranches, keepMemory: keep.keepMemory }),
      rollback: () =>
        batch(() => {
          ctx.S.projects = before;
          reconcileMock(ctx);
        }),
    });
  });
  if (!done) return false;
  forgetProject(ctx, project);
  toast(ctx, "Project removed");
  return true;
}
