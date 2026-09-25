/**
 * The CI state, CI age, month cost, and workflow runs the prototype's three projects had. The daemon
 * has none of them until a later phase, so a project from the daemon has none, and the screens show
 * an honest empty state. The unit tests that draw Home's CI health and the cost tiles need a project
 * that has them, so this puts them on the fixture projects, and only there.
 */
import type { State } from "../state-types";
import type { Project } from "../types";

type CiFields = Pick<Project, "ci" | "ciAgo" | "monthBase" | "runs">;

const PROTOTYPE_CI: Record<string, CiFields> = {
  api: {
    ci: "passed",
    ciAgo: 38,
    monthBase: 61.2,
    runs: [
      { wf: "test", st: "passed", ago: 38 },
      { wf: "lint", st: "passed", ago: 38 },
      { wf: "release", st: "cancelled", ago: 310 },
    ],
  },
  web: {
    ci: "running",
    ciAgo: 2,
    monthBase: 48.9,
    runs: [
      { wf: "ci", st: "running", ago: 2 },
      { wf: "e2e", st: "passed", ago: 95 },
      { wf: "deploy-preview", st: "passed", ago: 95 },
    ],
  },
  mobile: {
    ci: "failed",
    ciAgo: 22,
    monthBase: 88.4,
    runs: [
      { wf: "android", st: "failed", ago: 22, pkg: "apps/android" },
      { wf: "ios", st: "passed", ago: 22, pkg: "apps/ios" },
      { wf: "packages", st: "passed", ago: 22, pkg: "packages/*" },
    ],
  },
};

/** Puts the prototype's CI fields on the projects of the store that have an entry. */
export function overlayPrototypeCi(S: State): void {
  for (const project of S.projects) {
    const fields = PROTOTYPE_CI[project.id];
    if (fields) Object.assign(project, structuredClone(fields));
  }
}
