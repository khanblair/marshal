import type { CISnapshot } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import type { ApiClient } from "~/data/api-client";
import { golden } from "~/data/testing/golden";
import type { Ctx } from "~/mock/context";
import { ciRun, projectCI } from "~/testing/fake-ci";
import { createFakeDaemon } from "~/testing/fake-daemon";
import { daemonProject, PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createSyncedMarshal, createTestMarshal } from "~/testing/test-store";
import { applyCISnapshot, ciSyncer } from "./ci";
import { applyProjectSnapshot } from "./projects";

// Section S21: the CI health of every project. The daemon answers one snapshot for every project at
// once and publishes that same snapshot on the home topic whenever a run changes, so a load and a
// `ci.updated` are the same one write to the projects already in the store. The snapshot the first
// tests read is the golden one the Go tests wrote, so the two sides cannot drift.

const snapshot = golden<CISnapshot>("ci-snapshot");

/** A store whose projects are the ones named: the golden snapshot's own project is web-dashboard. */
function storeOf(...ids: string[]): Ctx {
  const ctx = contextOf(createTestMarshal());
  applyProjectSnapshot(
    ctx,
    ids.map((id) => daemonProject({ id })),
  );
  return ctx;
}

const projectOf = (ctx: Ctx, id: string) => {
  const project = ctx.S.projects.find((one) => one.id === id);
  if (!project) throw new Error(`the store has no project ${id}`);
  return project;
};

const homeEvent = (type: string, data: unknown) =>
  ({ seq: 1, topic: "home", type, at: "2026-09-27T09:30:00.000Z", data }) as never;

describe("the CI health section", () => {
  it("is section S21, follows the home topic, and loads the daemon's whole snapshot", async () => {
    expect(ciSyncer.section).toBe("S21");
    expect(ciSyncer.topics).toEqual(["home"]);
    const asked: string[] = [];
    const api = {
      ciSnapshot: async () => {
        asked.push("ciSnapshot");
        return snapshot;
      },
    } as unknown as ApiClient;
    expect(await ciSyncer.load(api, contextOf(createTestMarshal()))).toBe(snapshot);
    expect(asked).toEqual(["ciSnapshot"]);
  });

  it("gives the default branch's state, age, and runs to the project that has them", () => {
    const ctx = storeOf("web-dashboard");
    ciSyncer.apply(ctx, snapshot);
    expect(projectOf(ctx, "web-dashboard")).toMatchObject({
      ci: "passed",
      ciAgo: 36,
      // The golden's two runs on a card's branch are the card's badge, and are left out here.
      runs: [{ wf: "ci packages/web", st: "passed", ago: 36 }],
    });
  });

  it("leaves a project the snapshot omits with no CI, so Home shows its empty state", () => {
    const ctx = storeOf("web-dashboard", "api-gateway");
    const api = projectOf(ctx, "api-gateway");
    api.ci = "failed";
    api.ciAgo = 3;
    api.runs = [{ wf: "ci", st: "failed", ago: 3 }];
    ciSyncer.apply(ctx, snapshot);
    expect(api.ci).toBeUndefined();
    expect(api.ciAgo).toBeUndefined();
    expect(api.runs).toBeUndefined();
  });

  it("applies the same snapshot twice without redrawing what did not change", () => {
    const ctx = storeOf("web-dashboard");
    ciSyncer.apply(ctx, snapshot);
    const runs = projectOf(ctx, "web-dashboard").runs;
    ciSyncer.apply(ctx, snapshot);
    // The second apply finds the same words, so the runs array is not replaced.
    expect(projectOf(ctx, "web-dashboard").runs).toBe(runs);
  });

  it("applies a ci.updated on the home topic, and ignores one that carries a project alone", () => {
    const ctx = storeOf("web-dashboard");
    ciSyncer.onEvent?.(ctx, homeEvent("ci.updated", { snapshot }));
    expect(projectOf(ctx, "web-dashboard").ci).toBe("passed");
    const emptied: CISnapshot = { projects: [], serverTime: snapshot.serverTime };
    ciSyncer.onEvent?.(ctx, homeEvent("ci.updated", { snapshot: emptied }));
    expect(projectOf(ctx, "web-dashboard").ci).toBeUndefined();
    // A project-topic event carries one project and no server time, so home's own event is the one
    // this section follows and the project one changes nothing here.
    ciSyncer.onEvent?.(ctx, homeEvent("ci.updated", { project: snapshot.projects[0] }));
    expect(projectOf(ctx, "web-dashboard").ci).toBeUndefined();
  });

  it("ignores an event of another kind, and one whose payload is not a snapshot", () => {
    const ctx = storeOf("web-dashboard");
    ciSyncer.apply(ctx, snapshot);
    ciSyncer.onEvent?.(ctx, homeEvent("card.updated", { card: {} }));
    expect(projectOf(ctx, "web-dashboard").ci).toBe("passed");
    ciSyncer.onEvent?.(ctx, homeEvent("ci.updated", { snapshot: "nope" }));
    expect(projectOf(ctx, "web-dashboard").ci).toBe("passed");
    ciSyncer.onEvent?.(ctx, homeEvent("ci.updated", null));
    expect(projectOf(ctx, "web-dashboard").ci).toBe("passed");
  });

  it("takes a project with no run on its default branch as its own status with no age", () => {
    const ctx = storeOf("api");
    applyCISnapshot(ctx, {
      projects: [
        projectCI(
          "api",
          [ciRun("api", { branch: "marshal/41-work", cardId: "card-41", status: "failed" })],
          "queued",
        ),
      ],
      serverTime: "2026-09-27T09:30:00.000Z",
    });
    expect(projectOf(ctx, "api")).toMatchObject({ ci: "queued", runs: [] });
    expect(projectOf(ctx, "api").ciAgo).toBeUndefined();
  });
});

describe("the CI health section against the fake daemon", () => {
  it("fills the store's projects from the daemon's own snapshot when it follows one", async () => {
    const d = createFakeDaemon({
      projects: PROTOTYPE_PROJECTS,
      ci: [
        projectCI("web", [
          ciRun("web", { branch: "main", workflow: "ci packages/web", status: "passed" }),
        ]),
      ],
    });
    try {
      const M = await createSyncedMarshal(d);
      const ctx = contextOf(M);
      const web = ctx.S.projects.find((one) => one.id === "web");
      expect(web).toMatchObject({ ci: "passed", runs: [{ wf: "ci packages/web", st: "passed" }] });
      expect(typeof web?.ciAgo).toBe("number");
      // A project the snapshot leaves out has none of it, which is Home's "not connected" state.
      expect(ctx.S.projects.find((one) => one.id === "api")?.ci).toBeUndefined();
    } finally {
      d.data.stop();
    }
  });
});
