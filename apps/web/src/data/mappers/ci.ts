import type { CISnapshot, CIState, CiRun as WireRun, ProjectCI } from "@marshal/protocol";
import { toMillis } from "./time";

/**
 * A project's CI health, in the words the screens use for it (section S21, docs/backend-checklist.md
 * B6.2 to B6.4). The daemon answers with every project's runs in one snapshot; this turns one
 * project's share of it into the fields Home and the CI health page draw: the state of its default
 * branch, how long ago that run was, and the workflow runs on that branch, newest first.
 *
 * A run on a card's own branch is left out here on purpose. The card carries its own CI badge
 * (docs/architecture.md section 9), and the CI health page draws that run from the card, so counting
 * it here as well would show it twice.
 */

const MS_PER_MINUTE = 60_000;
/** The branch a run is on when the store does not know the project's default branch. */
const FALLBACK_BRANCH = "main";

/** One workflow run on a project's default branch, as the screens show it. */
interface ProjectWorkflowRun {
  /** The workflow's name as the forge names it, with its package for a monorepo, such as "ci packages/web". */
  wf: string;
  st: CIState;
  /** Whole minutes since the run last changed, counted from the daemon's own time. */
  ago: number;
}

/** One project's CI health as the screens keep it: its default branch's state, how long ago, and its runs. */
export interface ProjectCIRow {
  projectId: string;
  /** The state of the project's default branch. */
  status: CIState;
  /** Minutes since the newest run on the default branch, or undefined when Marshal knows of none. */
  ago: number | undefined;
  /** The default branch's workflow runs, newest first. */
  runs: ProjectWorkflowRun[];
}

/** Whole minutes from the daemon's time to a run's last change, never negative. */
function minutesSince(nowMs: number, updatedAt: string): number {
  return Math.max(0, Math.round((nowMs - toMillis(updatedAt)) / MS_PER_MINUTE));
}

/** One project's share of the snapshot: the runs on its default branch, newest first, and its state. */
function toProjectCIRow(project: ProjectCI, defaultBranch: string, nowMs: number): ProjectCIRow {
  const runs = project.runs
    .filter((run: WireRun) => run.branch === defaultBranch)
    .sort((a, b) => toMillis(b.updatedAt) - toMillis(a.updatedAt))
    .map((run: WireRun) => ({
      wf: run.workflow,
      st: run.status,
      ago: minutesSince(nowMs, run.updatedAt),
    }));
  return {
    projectId: project.projectId,
    status: project.status,
    ago: runs.length > 0 ? runs[0]?.ago : undefined,
    runs,
  };
}

/**
 * One row for each project the snapshot has, in the snapshot's own order. `branches` says which
 * branch is a project's default one, because the snapshot names a branch on each run and the store
 * is where each project's default branch is known.
 */
export function toProjectCIRows(
  snapshot: CISnapshot,
  branches: ReadonlyMap<string, string | undefined>,
): ProjectCIRow[] {
  const nowMs = toMillis(snapshot.serverTime);
  return snapshot.projects.map((project) =>
    toProjectCIRow(project, branches.get(project.projectId) ?? FALLBACK_BRANCH, nowMs),
  );
}
