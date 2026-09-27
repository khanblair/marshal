import type { CISnapshot } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { ciRun, projectCI } from "~/testing/fake-ci";
import { golden } from "../testing/golden";
import { toProjectCIRows } from "./ci";

/*
 * A project's CI health as the screens draw it (section S21, B6.2 to B6.4). The daemon answers every
 * project's runs in one snapshot; this turns one project's share of it into the state of its default
 * branch, how long ago that was, and the runs on that branch. The snapshot the first test reads is
 * the golden one the Go tests wrote, so the two sides cannot drift.
 */

/** The daemon's own snapshot, written by its Go tests. */
const snapshot = golden<CISnapshot>("ci-snapshot");

/** The default branch of the one project the golden names. Every fixture project defaults to main. */
const branches = new Map<string, string | undefined>([["web-dashboard", "main"]]);

describe("toProjectCIRows", () => {
  it("takes the default branch's state, age, and runs, with the age counted from the daemon's time", () => {
    // The golden's one run on main last changed at 08:54 and the daemon answered at 09:30.
    expect(toProjectCIRows(snapshot, branches)).toEqual([
      {
        projectId: "web-dashboard",
        status: "passed",
        ago: 36,
        runs: [{ wf: "ci packages/web", st: "passed", ago: 36 }],
      },
    ]);
  });

  it("leaves out a run on a card's own branch, which the card's own badge draws", () => {
    const [row] = toProjectCIRows(snapshot, branches);
    // The golden carries two runs on a card's branch, and neither is the project's own.
    expect(snapshot.projects[0]?.runs).toHaveLength(3);
    expect(row?.runs.map((run) => run.wf)).toEqual(["ci packages/web"]);
  });

  it("orders several runs on the default branch newest first", () => {
    const project = projectCI(
      "api",
      [
        ciRun("api", { workflow: "lint", branch: "main", updatedAt: "2026-09-27T09:00:00.000Z" }),
        ciRun("api", { workflow: "ci", branch: "main", updatedAt: "2026-09-27T09:20:00.000Z" }),
        ciRun("api", { workflow: "docs", branch: "main", updatedAt: "2026-09-27T08:00:00.000Z" }),
      ],
      "passed",
    );
    const rows = toProjectCIRows(
      { projects: [project], serverTime: "2026-09-27T09:30:00.000Z" },
      new Map([["api", "main"]]),
    );
    expect(rows[0]?.runs.map((run) => run.wf)).toEqual(["ci", "lint", "docs"]);
    expect(rows[0]?.ago).toBe(10);
  });

  it("never counts a negative age when a run is ahead of the daemon's clock", () => {
    const project = projectCI("api", [
      ciRun("api", { branch: "main", updatedAt: "2026-09-27T10:00:00.000Z" }),
    ]);
    const rows = toProjectCIRows(
      { projects: [project], serverTime: "2026-09-27T09:30:00.000Z" },
      new Map([["api", "main"]]),
    );
    expect(rows[0]?.ago).toBe(0);
  });

  it("treats main as the default branch of a project the store knows nothing about", () => {
    const rows = toProjectCIRows(snapshot, new Map());
    expect(rows[0]).toMatchObject({ ago: 36 });
  });

  it("has no age, and keeps the daemon's status, for a project with no run on its default branch", () => {
    const project = projectCI(
      "api",
      [ciRun("api", { branch: "marshal/41-work", cardId: "card-41", status: "failed" })],
      "queued",
    );
    const rows = toProjectCIRows(
      { projects: [project], serverTime: "2026-09-27T09:30:00.000Z" },
      new Map([["api", "main"]]),
    );
    expect(rows[0]).toEqual({ projectId: "api", status: "queued", ago: undefined, runs: [] });
  });

  it("answers one row per project, in the snapshot's own order", () => {
    const rows = toProjectCIRows(
      {
        projects: [projectCI("mobile"), projectCI("api")],
        serverTime: "2026-09-27T09:30:00.000Z",
      },
      new Map(),
    );
    expect(rows.map((row) => row.projectId)).toEqual(["mobile", "api"]);
  });

  it("answers no rows for a snapshot with nothing in it, which is a daemon that knows no project", () => {
    expect(
      toProjectCIRows({ projects: [], serverTime: "2026-09-27T09:30:00.000Z" }, branches),
    ).toEqual([]);
  });
});
