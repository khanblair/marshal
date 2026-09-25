import { type Card, type CiState, M, type Project } from "~/mock";
import { cardLabel } from "~/mock/card-key";

/* CI rows of the Home card and of the CI health page. */

const MINUTES_PER_HOUR = 60;
const MINUTES_PER_DAY = 1440;
const MS_PER_MINUTE = 60_000;

/** The `ago` of a project's main branch as Home words it: minutes under an hour, else hours. Empty when unknown. */
export function homeCiAgo(minutes: number | undefined): string {
  if (!minutes) return "";
  return minutes < MINUTES_PER_HOUR
    ? `${minutes} min ago`
    : `${Math.round(minutes / MINUTES_PER_HOUR)} h ago`;
}

/** The `ago` of a run on the CI health page: minutes, hours, then days. */
export function runAgo(minutes: number): string {
  if (minutes < MINUTES_PER_HOUR) return `${minutes} min ago`;
  if (minutes < MINUTES_PER_DAY) return `${Math.round(minutes / MINUTES_PER_HOUR)} h ago`;
  return `${Math.round(minutes / MINUTES_PER_DAY)} days ago`;
}

/** The state shown for a project's main branch. Unknown states look queued, as in the design. */
export const ciInfo = (state: CiState) => M.CI[state] ?? M.CI.queued;

export interface CiRun {
  /** Workflow with its package, or the branch of a card. */
  name: string;
  /** `main`, or the card the run belongs to. */
  where: string;
  state: CiState;
  minutesAgo: number;
  open: () => void;
}

/** A project that has CI data. */
export type ProjectWithCi = Project & { ci: CiState };

/** True when the project has CI data. A project from the daemon has none until GitHub is connected. */
export const hasCi = (project: Project): project is ProjectWithCi => project.ci !== undefined;

function workflowRuns(project: Project): CiRun[] {
  return (project.runs ?? []).map((run) => ({
    name: run.wf + (run.pkg ? ` ${run.pkg}` : ""),
    where: "main",
    state: run.st,
    minutesAgo: run.ago,
    open: () => M.go("project", project.id, "board"),
  }));
}

function cardRun(card: Card, state: CiState): CiRun {
  return {
    name: card.branch ?? "",
    where: `${cardLabel(card)} ${card.title}`,
    state,
    minutesAgo: Math.max(1, Math.round((M.now() - card.upd) / MS_PER_MINUTE)),
    open: () => M.openCard(card.id),
  };
}

function cardRuns(project: Project): CiRun[] {
  return M.cardsOf(project.id).flatMap((card) => (card.ci ? [cardRun(card, card.ci)] : []));
}

/** The project's workflow runs on main and its cards' runs, newest first. */
export function runsOf(project: Project): CiRun[] {
  return [...workflowRuns(project), ...cardRuns(project)].sort(
    (a, b) => a.minutesAgo - b.minutesAgo,
  );
}
