import { type CISnapshot, EventTypeCIUpdated, type Event as WireEvent } from "@marshal/protocol";
import { batch } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import { type ProjectCIRow, toProjectCIRows } from "~/data/mappers/ci";
import type { Ctx } from "~/mock/context";
import type { Project } from "~/mock/types";
import type { Syncer } from "./syncer";

/**
 * Section S21: the CI health of every project (docs/backend-checklist.md B6.2 to B6.4,
 * docs/architecture.md sections 9 and 11.2). Home's CI list and its view-all page both read the
 * projects in the store, so this fills the three fields the daemon is now the source of (`ci`,
 * `ciAgo`, and `runs`) and leaves everything else as it is.
 *
 * The daemon publishes `ci.updated` with the whole snapshot on the home topic, so one subscription
 * keeps the section current; a project-topic event carries only one project and no server time, and
 * is not needed here because the home one already describes the same change in full.
 */

/** The default branch of each project, which is what a run must be on to count as the project's. */
function branchesOf(ctx: Ctx): Map<string, string | undefined> {
  return new Map(ctx.S.projects.map((project) => [project.id, project.branch]));
}

const sameRuns = (
  a: readonly { wf: string; st: string; ago: number }[] | undefined,
  b: readonly { wf: string; st: string; ago: number }[],
): boolean => {
  if (!a || a.length !== b.length) return false;
  return a.every((run, i) => run.wf === b[i]?.wf && run.st === b[i]?.st && run.ago === b[i]?.ago);
};

/** Writes one project's CI fields, one at a time, so nothing redraws that did not change. */
function writeCi(project: Project, row: ProjectCIRow): void {
  if (project.ci !== row.status) project.ci = row.status;
  if (project.ciAgo !== row.ago) project.ciAgo = row.ago;
  const runs = row.runs.map((run) => ({ wf: run.wf, st: run.st, ago: run.ago }));
  if (!sameRuns(project.runs, runs)) project.runs = runs;
}

/** Takes the CI fields off a project and shows Home's honest empty state for it instead. */
function clearCi(project: Project): void {
  if (project.ci !== undefined) project.ci = undefined;
  if (project.ciAgo !== undefined) project.ciAgo = undefined;
  if (project.runs !== undefined) project.runs = undefined;
}

/**
 * Makes the store's CI fields the daemon's: every project in the snapshot gets its default branch's
 * state, age, and runs, and a project the snapshot leaves out has none. Applying the same snapshot
 * twice changes nothing.
 */
export function applyCISnapshot(ctx: Ctx, snapshot: CISnapshot): void {
  const rows = toProjectCIRows(snapshot, branchesOf(ctx));
  const known = new Set(rows.map((row) => row.projectId));
  batch(() => {
    for (const row of rows) {
      const project = ctx.S.projects.find((p) => p.id === row.projectId);
      if (project) writeCi(project, row);
    }
    for (const project of ctx.S.projects) if (!known.has(project.id)) clearCi(project);
  });
}

/** The snapshot inside a `ci.updated` event's data, or null when the event carries a single project. */
function snapshotOf(data: unknown): CISnapshot | null {
  if (!isRecord(data) || !isRecord(data.snapshot)) return null;
  return data.snapshot as unknown as CISnapshot;
}

export const ciSyncer: Syncer<CISnapshot> = {
  section: "S21",
  topics: ["home"],
  load: (api: ApiClient) => api.ciSnapshot(),
  apply: applyCISnapshot,
  onEvent(ctx: Ctx, event: WireEvent): void {
    if (event.type !== EventTypeCIUpdated) return;
    const snapshot = snapshotOf(event.data);
    if (snapshot) applyCISnapshot(ctx, snapshot);
  },
};
