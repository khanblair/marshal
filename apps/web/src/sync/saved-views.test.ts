import type { Preferences, SavedView } from "@marshal/protocol";
import { afterEach, describe, expect, it, vi } from "vitest";
import { golden } from "~/data/testing/golden";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { withMobileRow } from "~/testing/fake-me";
import { wireSavedView } from "~/testing/fake-saved-views";
import { PROTOTYPE_PROJECTS, wireProject } from "~/testing/projects";
import {
  contextOf,
  createSyncedMarshal,
  createTestMarshal,
  DAEMON_PERSON,
  MOCK_PERSON,
} from "~/testing/test-store";
import { savedViewsSyncer } from "./saved-views";

const saved = golden<Preferences>("preferences");
const needsMe = golden<SavedView>("saved-view");
const byRole = wireSavedView({
  id: "01M3C107JB041061050R3GG2V2",
  projectId: "api",
  name: "By role",
  filters: [],
  swimlane: "role",
});

let daemon: FakeDaemon | null = null;
afterEach(() => {
  vi.useRealTimers();
  daemon?.data.stop();
  daemon = null;
});

/** A daemon that already has the monorepo's row, so nothing is sent at load. See `withMobileRow`. */
const open = (options: Parameters<typeof createFakeDaemon>[0] = {}): FakeDaemon => {
  daemon = createFakeDaemon({
    projects: PROTOTYPE_PROJECTS,
    ...options,
    preferences: withMobileRow(options.preferences),
  });
  return daemon;
};

async function synced(d: FakeDaemon) {
  const M = await createSyncedMarshal(d, { sections: DAEMON_PERSON });
  vi.useFakeTimers();
  return { M, ctx: contextOf(M) };
}

const names = (M: { S: { savedViews: Record<string, { name: string }[]> } }, pid: string) =>
  (M.S.savedViews[pid] ?? []).map((view) => view.name);
const patches = (d: FakeDaemon): unknown[] => d.bodies("PATCH /v1/me/preferences");
const wait = (ms: number) => vi.advanceTimersByTimeAsync(ms);

describe("the saved views section", () => {
  it("is section S6a, follows each project's topic, and has no topic of its own", () => {
    expect(savedViewsSyncer.section).toBe("S6a");
    expect(savedViewsSyncer.topics).toEqual([]);
    expect(savedViewsSyncer.projectTopics?.("api")).toEqual(["project:api"]);
  });
});

describe("loading the saved views", () => {
  it("lists the daemon's views after the client's All cards, for each project", async () => {
    const d = open({ savedViews: [needsMe, byRole] });
    const { M } = await synced(d);
    expect(names(M, "api")).toEqual(["All cards", "Needs me", "By role"]);
    expect(names(M, "web")).toEqual(["All cards"]);
    expect(M.S.savedViews.api?.[1]).toMatchObject({
      id: needsMe.id,
      f: [{ k: "status", v: "needs" }],
      swim: "none",
    });
  });

  it("has only All cards when the daemon has no view at all, and shows it in use", async () => {
    const d = open();
    const { M } = await synced(d);
    for (const pid of ["api", "web", "mobile"]) expect(names(M, pid)).toEqual(["All cards"]);
    expect(M.S.savedView.api).toBe("All cards");
    expect(M.S.savedViews.api?.[0]?.id).toBeUndefined();
  });

  it("is right with the prototype's own views on the daemon: All cards in every project and By package in mobile", async () => {
    const seeded = (projectId: string, name: string, swimlane: SavedView["swimlane"], n: number) =>
      wireSavedView({
        id: `01M3C107JB041061050R3GG3${n}`,
        projectId,
        name,
        filters: [],
        swimlane,
      });
    const views = [
      seeded("api", "All cards", "none", 1),
      seeded("web", "All cards", "none", 2),
      seeded("mobile", "By package", "package", 3),
      seeded("mobile", "All cards", "none", 4),
    ];
    const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, savedViews: views });
    daemon = d;
    const { M } = await synced(d);
    expect(names(M, "api")).toEqual(["All cards"]);
    expect(names(M, "mobile")).toEqual(["By package", "All cards"]);
    // The client's own start is in use, and that is using none: no view is named to the daemon.
    for (const pid of ["api", "web", "mobile"]) expect(M.S.savedView[pid]).toBe("All cards");
    expect(M.S.swim.mobile).toBe("package");
    await wait(5000);
    // The only thing sent is the monorepo's package swimlane, once (the daemon's default is none).
    expect(patches(d)).toEqual([{ projects: { mobile: { swimlane: "package" } } }]);
    expect(d.me.preferences.projects.mobile).toMatchObject({
      swimlane: "package",
      savedViewId: null,
    });
    expect(M.S.swim.mobile).toBe("package");
  });

  it("sends no view for a daemon's All cards chosen from the menu, since that is using none", async () => {
    const own = wireSavedView({
      id: "01M3C107JB041061050R3GG2V3",
      projectId: "api",
      name: "All cards",
      filters: [],
    });
    const d = open({ savedViews: [own, needsMe] });
    const { M } = await synced(d);
    M.go("project", "api", "board");
    M.applyView("Needs me");
    await wait(200);
    expect(d.me.preferences.projects.api?.savedViewId).toBe(needsMe.id);
    M.applyView("All cards");
    await wait(200);
    expect(patches(d).at(-1)).toEqual({ projects: { api: { filters: [], savedViewId: "" } } });
    expect(d.me.preferences.projects.api?.savedViewId).toBeNull();
    expect(M.S.savedView.api).toBe("All cards");
  });

  it("does not list All cards twice when the daemon has its own", async () => {
    const own = wireSavedView({
      id: "01M3C107JB041061050R3GG2V3",
      projectId: "api",
      name: "All cards",
      filters: [],
      swimlane: "none",
    });
    const d = open({ savedViews: [own, needsMe] });
    const { M } = await synced(d);
    expect(names(M, "api")).toEqual(["All cards", "Needs me"]);
    expect(M.S.savedViews.api?.[0]?.id).toBe(own.id);
  });

  it("shows none of the prototype's own seeded views, only the daemon's", async () => {
    const d = open();
    const { M } = await synced(d);
    expect(JSON.stringify(M.S.savedViews)).not.toContain("Claude Code by role");
    expect(JSON.stringify(M.S.savedViews)).not.toContain("api-client only");
  });

  it("has only the client's All cards in a store with no daemon, and none of the prototype's seeded views", () => {
    const M = createTestMarshal({ sections: DAEMON_PERSON, data: null });
    for (const pid of ["api", "web", "mobile"]) expect(names(M, pid)).toEqual(["All cards"]);
    expect(M.S.savedView).toEqual({ api: "All cards", web: "All cards", mobile: "All cards" });
  });

  it("keeps the prototype's seeded views while the section is the mock's", () => {
    const M = createTestMarshal({ sections: MOCK_PERSON, data: null });
    expect(names(M, "api")).toEqual(["All cards", "Needs me", "Claude Code by role"]);
    expect(M.S.savedView.mobile).toBe("By package");
  });

  it("subscribes to each project's topic", async () => {
    const d = open();
    await synced(d);
    const topics = d.sockets.last().hellos().at(-1)?.subscribe as string[];
    expect(topics).toEqual(
      expect.arrayContaining(["project:api", "project:web", "project:mobile"]),
    );
  });
});

describe("saved_view.updated", () => {
  const update = (d: FakeDaemon, views: SavedView[]) =>
    d.emit("project:api", "saved_view.updated", { projectId: "api", views });

  it("replaces the project's list with the daemon's, and changes nothing the second time", async () => {
    const d = open();
    const { M } = await synced(d);
    update(d, [needsMe, byRole]);
    update(d, [needsMe, byRole]);
    expect(names(M, "api")).toEqual(["All cards", "Needs me", "By role"]);
    expect(names(M, "web")).toEqual(["All cards"]);
  });

  it("ends the use of a view that was deleted, without saying so to the daemon", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    const { M } = await synced(d);
    expect(M.S.savedView.api).toBe("Needs me");
    // What the daemon does for a delete: the database clears the person's saved view, and only the list is sent.
    d.me.preferences.projects.api!.savedViewId = null;
    update(d, []);
    expect(names(M, "api")).toEqual(["All cards"]);
    expect(M.S.savedView.api).toBeNull();
    expect(M.S.filters.api).toEqual([{ k: "status", v: "needs" }]);
    await wait(5000);
    expect(patches(d)).toEqual([]);
  });

  it("keeps a view in use when the daemon renames it, and does not save that", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    const { M } = await synced(d);
    update(d, [{ ...needsMe, name: "Waiting on me" }]);
    expect(M.S.savedView.api).toBe("Waiting on me");
    await wait(5000);
    expect(patches(d)).toEqual([]);
  });

  it("keeps All cards in use while the list still has it", async () => {
    const d = open();
    const { M } = await synced(d);
    update(d, [needsMe]);
    expect(M.S.savedView.api).toBe("All cards");
  });

  it("ignores an event without a project or without a list", async () => {
    const d = open({ savedViews: [needsMe] });
    const { M } = await synced(d);
    d.emit("project:api", "saved_view.updated", { views: [] });
    d.emit("project:api", "saved_view.updated", { projectId: "api" });
    expect(names(M, "api")).toEqual(["All cards", "Needs me"]);
  });
});

describe("a project that is removed", () => {
  it("takes its saved views with it, and a project added again starts with All cards", async () => {
    const d = open({ savedViews: [needsMe] });
    const { M } = await synced(d);
    d.emit("home", "project.removed", { projectId: "api" });
    expect(M.S.savedViews.api).toBeUndefined();
    d.emit("home", "project.created", {
      project: wireProject({ id: "api", name: "api", path: "/a" }),
    });
    expect(names(M, "api")).toEqual(["All cards"]);
    expect(M.S.savedView.api).toBe("All cards");
  });
});

describe("saving a view", () => {
  it("saves the filters and swimlane on screen, puts the view in use, and saves that too", async () => {
    const d = open();
    const { M } = await synced(d);
    M.go("project", "api", "board");
    M.addFilter("role", "Tester");
    M.S.swim.api = "agent";
    M.set({ menu: "views" });
    M.saveView("Testers");
    await wait(100);
    expect(d.bodies("POST /v1/projects/api/saved-views")).toEqual([
      { name: "Testers", filters: [{ key: "role", value: "Tester" }], swimlane: "agent" },
    ]);
    const view = d.me.savedViews[0];
    expect(names(M, "api")).toEqual(["All cards", "Testers"]);
    expect(M.S.savedView.api).toBe("Testers");
    expect(M.S.savedViews.api?.[1]?.id).toBe(view?.id);
    expect(M.S.menu).toBeNull();
    expect(M.S.toasts.map((toast) => toast.msg)).toEqual(["View saved"]);
    await wait(300);
    expect(d.me.preferences.projects.api?.savedViewId).toBe(view?.id);
  });

  it("replaces the view of a name that is already used, keeping its id, in one place in the list", async () => {
    const d = open({ savedViews: [needsMe, byRole] });
    const { M } = await synced(d);
    M.go("project", "api", "board");
    M.addFilter("label", "ui");
    M.saveView("needs me");
    await wait(100);
    expect(names(M, "api")).toEqual(["All cards", "By role", "needs me"]);
    expect(M.S.savedViews.api?.at(-1)?.id).toBe(needsMe.id);
    expect(M.S.savedViews.api?.at(-1)?.f).toEqual([{ k: "label", v: "ui" }]);
    expect(d.me.savedViews.map((view) => view.name)).toEqual(["By role", "needs me"]);
    expect(M.S.savedView.api).toBe("needs me");
  });

  it("replaces the client's All cards when a view of that name is saved", async () => {
    const d = open();
    const { M } = await synced(d);
    M.go("project", "api", "board");
    M.addFilter("role", "Tester");
    M.saveView("All cards");
    await wait(100);
    expect(names(M, "api")).toEqual(["All cards"]);
    expect(M.S.savedViews.api?.[0]?.f).toEqual([{ k: "role", v: "Tester" }]);
  });

  it("shows the daemon's sentence and changes nothing when it refuses the name", async () => {
    const d = open();
    const { M } = await synced(d);
    M.go("project", "api", "board");
    M.set({ menu: "views" });
    M.saveView("x".repeat(70));
    await wait(100);
    expect(M.S.toasts.map((toast) => toast.msg)).toEqual([
      "Saved view names can have at most 60 characters.",
    ]);
    expect(names(M, "api")).toEqual(["All cards"]);
    expect(M.S.savedView.api).toBe("All cards");
    expect(M.S.menu).toBe("views");
  });

  it("shows the daemon's sentence at the limit of 50", async () => {
    const many = Array.from({ length: 50 }, (_, i) =>
      wireSavedView({
        id: `01M3C107JB041061050R3GG${String(100 + i)}`,
        projectId: "api",
        name: `View ${i}`,
      }),
    );
    const d = open({ savedViews: many });
    const { M } = await synced(d);
    M.go("project", "api", "board");
    M.saveView("One more");
    await wait(100);
    expect(M.S.toasts.at(-1)?.msg).toBe(
      "A project can have at most 50 saved views. Delete one and try again.",
    );
  });

  it("does nothing for an empty name, or with no project open, and asks the daemon nothing", async () => {
    const d = open();
    const { M } = await synced(d);
    M.go("project", "api", "board");
    M.saveView("");
    M.go("home");
    M.S.route.pid = null;
    M.saveView("Something");
    await wait(100);
    expect(d.routes().filter((route) => route.startsWith("POST /v1/projects"))).toEqual([]);
  });

  it("saves for the project that was open when it was asked, though another opens before the answer", async () => {
    const d = open();
    const { M } = await synced(d);
    M.go("project", "api", "board");
    const release = d.holdNext("POST /v1/projects/api/saved-views");
    M.saveView("Mine");
    M.go("project", "web", "board");
    release();
    await wait(100);
    expect(names(M, "api")).toEqual(["All cards", "Mine"]);
    expect(names(M, "web")).toEqual(["All cards"]);
    expect(M.S.savedView.api).toBe("Mine");
  });
});
