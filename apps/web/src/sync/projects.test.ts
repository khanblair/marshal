import type { Event as WireEvent } from "@marshal/protocol";
import { createEffect, createRoot } from "solid-js";
import { describe, expect, it } from "vitest";
import { toDaemonProject } from "~/data/mappers/project";
import { golden } from "~/data/testing/golden";
import { createContext } from "~/mock/context";
import { feed, notice } from "~/mock/engine";
import { daemonProject, PROTOTYPE_PROJECTS } from "~/testing/projects";
import { MOCK_CARDS, MOCK_PERSON_SECTIONS, testEnv } from "~/testing/test-store";
import {
  applyProject,
  applyProjectRemoved,
  applyProjectSnapshot,
  ensureProjectState,
  projectsSyncer,
} from "./projects";

/** A store with nothing from the daemon yet, as the app has before its first answer. */
// The reservoir is the mock's own machinery, so these tests say out loud that S5a, S17, and S20 are
// on the mock and keep testing it whatever the register says.
const emptyStore = () =>
  createContext(
    testEnv({
      hash: "#nosim",
      sections: { ...MOCK_CARDS, ...MOCK_PERSON_SECTIONS, S17: "mock", S20: "mock" },
    }),
  );
const prototype = PROTOTYPE_PROJECTS.map(toDaemonProject);
const only = (...ids: string[]) => prototype.filter((p) => ids.includes(p.id));
const cardProjects = (ctx: ReturnType<typeof emptyStore>) => [
  ...new Set(ctx.S.cards.map((c) => c.p)),
];

describe("a store before the daemon has answered", () => {
  it("shows no project and none of the mock records that belong to one", () => {
    const { S, hidden } = emptyStore();
    expect(S.projects).toEqual([]);
    expect(S.cards).toEqual([]);
    expect(S.chats).toEqual({});
    expect(S.notices).toEqual([]);
    expect(S.feed).toEqual([]);
    // The records are not lost: they wait for their project.
    expect(hidden.cards.length).toBeGreaterThan(20);
    expect(Object.keys(hidden.chats).sort()).toEqual(["api", "mobile", "web"]);
  });
});

describe("applyProjectSnapshot", () => {
  it("adds the projects in the daemon's order and brings their mock records with them", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, prototype);
    expect(ctx.S.projects.map((p) => p.id)).toEqual(["api", "web", "mobile"]);
    expect(ctx.S.cards).toHaveLength(29);
    expect(ctx.hidden.cards).toEqual([]);
    expect(ctx.S.notices.map((n) => n.id)).toEqual(["n1", "n2", "n3"]);
    expect(Object.keys(ctx.S.chats).sort()).toEqual(["api", "mobile", "web"]);
  });

  it("fills a project from the daemon: name, language, path, branch, dev command, lock, packages", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, prototype);
    expect(ctx.S.projects.find((p) => p.id === "mobile")).toMatchObject({
      name: "mobile-app",
      lang: "Monorepo",
      path: "~/code/mobile-app",
      branch: "main",
      dev: "pnpm dev",
      lockBypass: false,
      packages: ["apps/ios", "apps/android", "packages/ui", "packages/auth", "packages/api-client"],
    });
    expect(ctx.S.projects.find((p) => p.id === "api")).toMatchObject({
      dev: "",
      packages: undefined,
    });
  });

  it("shows only the mock records of the projects that exist", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, only("web"));
    expect(cardProjects(ctx)).toEqual(["web"]);
    expect(Object.keys(ctx.S.chats)).toEqual(["web"]);
    // The sleep notice lists cards of api and web, so it stays; the notices about mobile go.
    expect(ctx.S.notices.map((n) => n.id)).toEqual(["n1"]);
    expect(ctx.S.feed.every((f) => f.pid === null || f.pid === "web")).toBe(true);
    expect(ctx.S.feed.some((f) => f.pid === "web")).toBe(true);
    expect(ctx.hidden.cards.every((c) => c.p !== "web")).toBe(true);
  });

  it("brings hidden records back when their project arrives later, and none for one they do not belong to", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, only("web"));
    applyProjectSnapshot(ctx, [...only("web"), daemonProject({ id: "my-service" })]);
    expect(cardProjects(ctx)).toEqual(["web"]);
    applyProjectSnapshot(ctx, [...only("web", "api"), daemonProject({ id: "my-service" })]);
    expect(cardProjects(ctx).sort()).toEqual(["api", "web"]);
    // A real project with no mock records shows the empty board and no chats.
    expect(ctx.S.cards.filter((c) => c.p === "my-service")).toEqual([]);
    expect(ctx.S.chats["my-service"]).toEqual([]);
  });

  it("changes nothing when the same list is applied again, and redraws nothing", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, prototype);
    const rows = [...ctx.S.projects];
    let runs = 0;
    const dispose = createRoot((stop) => {
      createEffect(() => {
        runs += 1;
        void ctx.S.projects.map((p) => `${p.id}${p.name}${p.dev}${p.packages?.join()}`);
        void ctx.S.cards.length;
      });
      return stop;
    });
    const before = runs;
    applyProjectSnapshot(ctx, prototype);
    applyProjectSnapshot(ctx, prototype);
    expect(runs).toBe(before);
    expect(ctx.S.projects.every((p, i) => p === rows[i])).toBe(true);
    dispose();
  });

  it("updates a project in place and leaves its mock-only fields alone", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, prototype);
    const api = ctx.S.projects[0];
    if (!api) throw new Error("no api project");
    api.ci = "passed";
    api.monthBase = 61.2;
    applyProjectSnapshot(ctx, [
      daemonProject({ id: "api", name: "gateway", devCommand: "go run ." }),
      ...only("web", "mobile"),
    ]);
    expect(ctx.S.projects[0]).toBe(api);
    expect(api).toMatchObject({ name: "gateway", dev: "go run .", ci: "passed", monthBase: 61.2 });
  });

  it("follows the daemon's order", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, prototype);
    applyProjectSnapshot(ctx, [...prototype].reverse());
    expect(ctx.S.projects.map((p) => p.id)).toEqual(["mobile", "web", "api"]);
  });

  it("forgets a project the daemon no longer has, with its mock records, once", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, prototype);
    applyProjectSnapshot(ctx, only("web", "mobile"));
    expect(ctx.S.projects.map((p) => p.id)).toEqual(["web", "mobile"]);
    expect(ctx.S.cards.some((c) => c.p === "api")).toBe(false);
    expect(ctx.hidden.cards.some((c) => c.p === "api")).toBe(false);
    expect(ctx.S.feed.filter((f) => f.text.includes("api-gateway was removed"))).toHaveLength(1);
    // It is gone for good: the same project coming back does not bring the old mock cards.
    applyProjectSnapshot(ctx, prototype);
    expect(ctx.S.cards.some((c) => c.p === "api")).toBe(false);
  });

  it("points the screen at a project that exists", () => {
    const ctx = emptyStore();
    expect(ctx.S.route.pid).toBeNull();
    applyProjectSnapshot(ctx, prototype);
    expect(ctx.S.route.pid).toBe("api");
    ctx.S.route = { page: "project", pid: "web", view: "board" };
    applyProjectSnapshot(ctx, only("api", "mobile"));
    expect(ctx.S.route).toEqual({ page: "home", pid: "api", view: "board" });
    applyProjectSnapshot(ctx, []);
    expect(ctx.S.route.pid).toBeNull();
  });

  it("closes the open card when its project goes", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, prototype);
    ctx.S.openId = "web#119";
    applyProjectSnapshot(ctx, only("api", "mobile"));
    expect(ctx.S.openId).toBeNull();
  });
});

describe("the events of the Home topic", () => {
  const events = golden<{ events: WireEvent[] }>("project-events").events;
  const [created, updated] = events;
  if (!created || !updated) throw new Error("the golden events changed");
  const apply = (ctx: ReturnType<typeof emptyStore>, event: WireEvent) => {
    projectsSyncer.onEvent?.(ctx, event);
  };

  it("adds a project from project.created, once, however many times the event arrives", () => {
    const ctx = emptyStore();
    apply(ctx, created);
    apply(ctx, created);
    expect(ctx.S.projects.map((p) => p.id)).toEqual(["web-dashboard"]);
    expect(ctx.S.filters["web-dashboard"]).toEqual([]);
  });

  it("changes a project from project.updated, in place", () => {
    const ctx = emptyStore();
    apply(ctx, created);
    const row = ctx.S.projects[0];
    apply(ctx, {
      ...updated,
      data: {
        project: { ...(updated.data as { project: object }).project, name: "Web", devCommand: "" },
      },
    });
    expect(ctx.S.projects).toHaveLength(1);
    expect(ctx.S.projects[0]).toBe(row);
    expect(row).toMatchObject({ name: "Web", dev: "" });
  });

  it("removes a project from project.removed, and says so in the feed once", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, prototype);
    const removed: WireEvent = { ...created, type: "project.removed", data: { projectId: "web" } };
    apply(ctx, removed);
    apply(ctx, removed);
    expect(ctx.S.projects.map((p) => p.id)).toEqual(["api", "mobile"]);
    expect(ctx.S.feed.filter((f) => f.text.startsWith("web-dashboard was removed"))).toHaveLength(
      1,
    );
  });

  it("ignores events of other kinds and data it cannot read", () => {
    const ctx = emptyStore();
    apply(ctx, { ...created, type: "card.created" });
    apply(ctx, { ...created, data: null });
    apply(ctx, { ...created, data: { project: 7 } });
    apply(ctx, { ...created, type: "project.removed", data: {} });
    expect(ctx.S.projects).toEqual([]);
  });
});

describe("applyProject and applyProjectRemoved", () => {
  it("adds a project at the end and updates it by id", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, only("api"));
    applyProject(ctx, daemonProject({ id: "billing", path: "~/code/billing" }));
    applyProject(ctx, daemonProject({ id: "billing", name: "Billing", path: "~/code/billing" }));
    expect(ctx.S.projects.map((p) => [p.id, p.name])).toEqual([
      ["api", "api-gateway"],
      ["billing", "Billing"],
    ]);
  });

  it("does nothing for a project that is not there", () => {
    const ctx = emptyStore();
    applyProjectRemoved(ctx, "ghost");
    expect(ctx.S.projects).toEqual([]);
    expect(ctx.S.feed).toEqual([]);
  });
});

describe("the screen state every project has", () => {
  it("gives a new project the mock's own defaults, once", () => {
    const ctx = emptyStore();
    applyProject(ctx, daemonProject({ id: "billing" }));
    const { S } = ctx;
    expect(S.filters.billing).toEqual([]);
    expect(S.query.billing).toBe("");
    expect(S.swim.billing).toBe("none");
    expect(S.savedViews.billing).toEqual([{ name: "All cards", f: [], swim: "none" }]);
    expect(S.savedView.billing).toBe("All cards");
    expect(S.lastView.billing).toBe("board");
    expect(S.chats.billing).toEqual([]);
    expect(S.limits.billing).toEqual({ day: 8, month: 120, awake: 6 });
  });

  it("swims by package in a monorepo", () => {
    const ctx = emptyStore();
    applyProject(
      ctx,
      daemonProject({ id: "platform", isMonorepo: true, packages: ["packages/a", "packages/b"] }),
    );
    expect(ctx.S.swim.platform).toBe("package");
  });

  it("never overwrites what a project already has", () => {
    const ctx = emptyStore();
    applyProject(ctx, daemonProject({ id: "billing" }));
    ctx.S.filters.billing = [{ k: "status", v: "needs" }];
    ctx.S.query.billing = "login";
    ctx.S.limits.billing = { day: 1, month: 2, awake: 3 };
    applyProject(ctx, daemonProject({ id: "billing", name: "Billing" }));
    applyProjectSnapshot(ctx, [daemonProject({ id: "billing" })]);
    expect(ctx.S.filters.billing).toEqual([{ k: "status", v: "needs" }]);
    expect(ctx.S.query.billing).toBe("login");
    expect(ctx.S.limits.billing).toEqual({ day: 1, month: 2, awake: 3 });
  });

  it("keeps the richer saved views the prototype's projects had", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, prototype);
    expect(ctx.S.savedViews.api?.map((v) => v.name)).toEqual([
      "All cards",
      "Needs me",
      "Claude Code by role",
    ]);
    expect(ctx.S.swim.mobile).toBe("package");
  });

  it("can be filled for a project made by hand", () => {
    const ctx = emptyStore();
    ensureProjectState(ctx.S, { id: "hand", packages: undefined });
    expect(ctx.S.lastView.hand).toBe("board");
  });
});

describe("mock notices and feed items about a project that is not there", () => {
  it("are not made", () => {
    const ctx = emptyStore();
    applyProjectSnapshot(ctx, only("api"));
    feed(ctx, { kind: "tool", text: "Ghost activity", pid: "mobile" });
    notice(ctx, { kind: "cost", pid: "mobile", text: "Ghost", sub: "" });
    feed(ctx, { kind: "tool", text: "Real activity", pid: "api" });
    feed(ctx, { kind: "brief", text: "About nobody", pid: null });
    expect(ctx.S.feed.map((f) => f.text)).toContain("Real activity");
    expect(ctx.S.feed.map((f) => f.text)).toContain("About nobody");
    expect(ctx.S.feed.map((f) => f.text)).not.toContain("Ghost activity");
    expect(ctx.S.notices.some((n) => n.kind === "cost")).toBe(false);
  });
});

describe("projectsSyncer", () => {
  it("is section S3 and follows the Home topic", () => {
    expect(projectsSyncer.section).toBe("S3");
    expect(projectsSyncer.topics).toEqual(["home"]);
  });
});
