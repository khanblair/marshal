/**
 * The CI health route of the fake daemon (docs/backend-checklist.md B6.2 to B6.4, section S21,
 * docs/architecture.md 9 and 11.2). One call answers every project's CI at once, so there is one
 * route and one answer: `GET /v1/ci` is the whole snapshot, and a project with no run of its own is
 * left out of it, the way the daemon leaves it out, so a screen shows "GitHub is not connected"
 * rather than an empty list.
 *
 * The snapshot is what a test seeds and the store hands back, and it carries the daemon's own time,
 * which is what every age on the screens is counted from. A run on a card's own branch is kept in
 * the snapshot too: it is the card's badge, and the app leaves it out of the project's row.
 */
import {
  type CISnapshot,
  type CIState,
  CIStatePassed,
  CIStateQueued,
  type CiRun,
  type ProjectCI,
  type Timestamp,
} from "@marshal/protocol";
import { type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";

/** The snapshot and the clock it is written from, as the fake daemon holds them. */
export interface CIStore {
  /** Every project Marshal has CI data for, in project order. */
  projects: ProjectCI[];
  /** Its clock, as the ISO string a snapshot's `serverTime` is written from. */
  now: () => string;
}

export interface FakeCIOptions {
  /** The projects that have CI data. None by default, so every project reads as not connected. */
  projects?: readonly ProjectCI[];
  /** Its clock, as an ISO string. The wall clock by default. */
  now?: () => string;
}

export function createCIStore(options: FakeCIOptions = {}): CIStore {
  return {
    projects: structuredClone([...(options.projects ?? [])]),
    now: options.now ?? (() => new Date().toISOString()),
  };
}

/** The answer to `GET /v1/ci`: the whole list, with the daemon's own time to count ages from. */
export function ciSnapshotOf(store: CIStore): CISnapshot {
  return { projects: structuredClone(store.projects), serverTime: store.now() };
}

/** Answers the CI route, or null when the request is not one. */
export function answerCIRoute(store: CIStore, request: FakeRequest): Response | null {
  const path = request.url.replace(/^https?:\/\/[^/]+/, "").split("?")[0];
  if (path === "/v1/ci" && request.method === "GET") return jsonAnswer(ciSnapshotOf(store));
  return null;
}

/** How many ids this module has handed out, so two runs a test seeds never share one. */
let madeRuns = 0;

/** What a test says about one seeded run. Everything left out is a passing run on `main`. */
export interface FakeRunFields {
  /** The run's opaque id. One made from the branch and workflow by default. */
  id?: string;
  /** The workflow's name, as the forge names it. "ci" by default. */
  workflow?: string;
  /** Where the run is. `passed` by default. */
  status?: CIState;
  /** The branch the run is on. `main` by default, which is every fixture project's default branch. */
  branch?: string;
  /** The card whose branch the run is on, or "" for a run on the default branch. */
  cardId?: string;
  /** When the run last changed. Now by default, so a screen counts it as just now. */
  updatedAt?: Timestamp;
}

/** One run a test seeds, with the fields the screens read and sensible ones for the rest. */
export function ciRun(projectId: string, fields: FakeRunFields = {}): CiRun {
  madeRuns += 1;
  const branch = fields.branch ?? "main";
  const workflow = fields.workflow ?? "ci";
  return {
    id: fields.id ?? `fakerun-${String(madeRuns)}-${branch}-${workflow}`,
    projectId,
    cardId: fields.cardId ?? "",
    branch,
    workflow,
    status: fields.status ?? CIStatePassed,
    url: "",
    // A run that has not started has no start time; nothing on the screens draws either one.
    startedAt: null,
    updatedAt: fields.updatedAt ?? new Date().toISOString(),
  };
}

/**
 * One project's CI health, from the runs a test seeded, newest first. The status is the newest run's
 * unless the test says otherwise, which is what makes a project `queued` with no run on its default
 * branch, the way the daemon reports a project it knows nothing about.
 */
export function projectCI(
  projectId: string,
  runs: readonly CiRun[] = [],
  status?: CIState,
): ProjectCI {
  const ordered = [...runs].sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt));
  return { projectId, status: status ?? ordered[0]?.status ?? CIStateQueued, runs: ordered };
}
