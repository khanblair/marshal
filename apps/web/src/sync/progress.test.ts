import type { Progress } from "@marshal/protocol";
import { afterEach, describe, expect, it } from "vitest";
import { golden } from "~/data/testing/golden";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createSyncedMarshal, DAEMON_PERSON, MOCK_PERSON } from "~/testing/test-store";
import { applyProgress, progressSyncer } from "./progress";

const pending = (step = 0): Progress => ({
  onboarding: { status: "pending", step, finishedAt: null },
  tutorial: { status: "pending", finishedAt: null },
});
const finished: Progress = {
  onboarding: { status: "done", step: 3, finishedAt: "2026-09-26T12:00:00.000Z" },
  tutorial: { status: "pending", finishedAt: null },
};

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});
const open = (options: Parameters<typeof createFakeDaemon>[0] = {}): FakeDaemon => {
  daemon = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, ...options });
  return daemon;
};

async function synced(d: FakeDaemon, sections = DAEMON_PERSON) {
  const M = await createSyncedMarshal(d, { sections });
  return { M, ctx: contextOf(M) };
}

describe("the progress section", () => {
  it("is section S31a and follows the me topic", () => {
    expect(progressSyncer.section).toBe("S31a");
    expect(progressSyncer.topics).toEqual(["me"]);
  });
});

describe("loading the progress", () => {
  it("shows onboarding at the step the daemon saved, and no tour yet", async () => {
    const d = open({ progress: pending(2) });
    const { M } = await synced(d);
    expect(M.S.onboarding).toBe(true);
    expect(M.S.obStep).toBe(2);
    expect(M.S.tour).toBeNull();
  });

  it("ends onboarding and starts the tour when onboarding was finished and the tour was not", async () => {
    const d = open({ progress: finished });
    const { M } = await synced(d);
    expect(M.S.onboarding).toBe(false);
    expect(M.S.tour).toEqual({ step: 0 });
  });

  it("starts no tour when it was finished or skipped", async () => {
    const d = open({
      progress: {
        ...finished,
        tutorial: { status: "skipped", finishedAt: "2026-09-26T12:00:00.000Z" },
      },
    });
    const { M } = await synced(d);
    expect(M.S.onboarding).toBe(false);
    expect(M.S.tour).toBeNull();
  });

  it("does not ask for the progress, and leaves onboarding alone, while the section is the mock's", async () => {
    const d = open({ progress: finished });
    const { M } = await synced(d, { ...DAEMON_PERSON, S31a: "mock" });
    expect(d.routes()).not.toContain("GET /v1/me/progress");
    expect(M.S.onboarding).toBe(true);
  });

  it("does not ask for it in a store that is all mock either", async () => {
    const d = open();
    await synced(d, MOCK_PERSON);
    expect(d.routes()).not.toContain("GET /v1/me/progress");
  });
});

describe("me.updated", () => {
  const update = (d: FakeDaemon, progress: Progress) =>
    d.emit("me", "me.updated", {
      profile: d.me.profile,
      preferences: d.me.preferences,
      progress,
    });

  it("follows a change from another device, and changes nothing the second time", async () => {
    const d = open({ progress: pending() });
    const { M } = await synced(d);
    update(d, finished);
    update(d, finished);
    expect(M.S.onboarding).toBe(false);
    expect(M.S.obStep).toBe(3);
    expect(M.S.tour).toEqual({ step: 0 });
  });

  it("keeps the step of a tour that is running, and ends it when the daemon says it is over", async () => {
    const d = open({ progress: finished });
    const { M } = await synced(d);
    M.S.tour = { step: 4 };
    update(d, finished);
    expect(M.S.tour).toEqual({ step: 4 });
    update(d, {
      ...finished,
      tutorial: { status: "done", finishedAt: "2026-09-26T13:00:00.000Z" },
    });
    expect(M.S.tour).toBeNull();
  });

  it("brings onboarding back when the daemon was reset", async () => {
    const d = open({ progress: finished, dev: true });
    const { M, ctx } = await synced(d);
    expect(M.S.onboarding).toBe(false);
    await ctx.env.data?.api.resetFirstLaunch();
    expect(M.S.onboarding).toBe(true);
    expect(M.S.obStep).toBe(0);
    expect(M.S.tour).toBeNull();
  });

  it("ignores an event without progress", async () => {
    const d = open({ progress: pending() });
    const { M } = await synced(d);
    d.emit("me", "me.updated", { profile: d.me.profile });
    expect(M.S.onboarding).toBe(true);
  });
});

describe("applying a snapshot", () => {
  it("writes nothing that is already so", async () => {
    const d = open({ progress: golden<Progress>("progress") });
    const { ctx } = await synced(d);
    applyProgress(ctx, golden<Progress>("progress"));
    expect(ctx.S.obStep).toBe(2);
  });
});
