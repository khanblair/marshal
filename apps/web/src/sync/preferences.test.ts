import type { Preferences, SavedView } from "@marshal/protocol";
import { afterEach, describe, expect, it, vi } from "vitest";
import { golden } from "~/data/testing/golden";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { emptyPreferences, withMobileRow } from "~/testing/fake-me";
import { PROTOTYPE_PROJECTS, wireProject } from "~/testing/projects";
import {
  contextOf,
  createSyncedMarshal,
  createTestMarshal,
  DAEMON_PERSON,
} from "~/testing/test-store";
import { forgetProjectPrefs, noteSavedViewGone, preferencesSyncer } from "./preferences";

const saved = golden<Preferences>("preferences");
const needsMe = golden<SavedView>("saved-view");

let daemon: FakeDaemon | null = null;
const others: FakeDaemon["data"][] = [];
afterEach(() => {
  vi.useRealTimers();
  daemon?.data.stop();
  for (const data of others.splice(0)) data.stop();
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

/** A store on the daemon, waited for until its first snapshots are in. The theme is applied through a spy. */
async function synced(d: FakeDaemon) {
  const applyTheme = vi.fn();
  const M = await createSyncedMarshal(d, { sections: DAEMON_PERSON, applyTheme });
  return { M, applyTheme, ctx: contextOf(M) };
}

const patches = (d: FakeDaemon): unknown[] => d.bodies("PATCH /v1/me/preferences");

describe("the preferences section", () => {
  it("is section S32, follows the me topic, and reads the preferences", () => {
    expect(preferencesSyncer.section).toBe("S32");
    expect(preferencesSyncer.topics).toEqual(["me"]);
  });

  it("subscribes the connection to the me topic beside home, once", async () => {
    const d = open();
    await synced(d);
    const topics = d.sockets.last().hellos().at(-1)?.subscribe as string[];
    expect(topics.filter((topic) => topic === "me")).toHaveLength(1);
    expect(topics).toContain("home");
  });
});

describe("loading the preferences", () => {
  it("puts the daemon's values in the store, in the store's own words", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    const { M, applyTheme } = await synced(d);
    expect(M.S.theme).toBe("dark");
    expect(applyTheme).toHaveBeenCalled();
    expect(M.S.listCols.pkg).toBe(true);
    expect(M.S.listCols.cost).toBe(false);
    expect(M.S.listCols.title).toBe(true);
    expect(M.S.sort.list).toEqual({ k: "id", dir: -1 });
    expect(M.S.sort.agents).toEqual({ k: "state", dir: 1 });
    expect(M.S.lastView.api).toBe("list");
    expect(M.S.filters.api).toEqual([{ k: "status", v: "needs" }]);
    expect(M.S.query.api).toBe("cache");
    expect(M.S.swim.api).toBe("role");
    expect(M.S.laneCollapsed["api:role:Tester"]).toBe(true);
    expect(M.S.showAllDone.api).toBe(true);
    expect(M.S.savedView.api).toBe("Needs me");
  });

  it("leaves a project with nothing saved as it starts: board, no filters, All cards", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    const { M } = await synced(d);
    expect(M.S.lastView.mobile).toBe("board");
    expect(M.S.swim.mobile).toBe("package");
    expect(M.S.savedView.mobile).toBe("All cards");
    expect(M.S.filters.mobile).toEqual([]);
  });

  it("shows All cards for a project whose saved preferences filter nothing and group nothing", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    const { M } = await synced(d);
    expect(M.S.savedView.web).toBe("All cards");
  });

  it("shows an unsaved view when the daemon has filters and no view in use", async () => {
    const prefs = structuredClone(saved);
    prefs.projects.api = { ...prefs.projects.api!, savedViewId: null };
    const d = open({ preferences: prefs });
    const { M } = await synced(d);
    expect(M.S.filters.api).toEqual([{ k: "status", v: "needs" }]);
    expect(M.S.savedView.api).toBeNull();
  });

  it("reads a saved view that is not on the project's list as none", async () => {
    const d = open({ preferences: saved });
    const { M } = await synced(d);
    expect(M.S.savedView.api).toBeNull();
  });

  it("sends nothing at load when the daemon has a row for every project it has anything to say for", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    await synced(d);
    vi.useFakeTimers();
    await vi.advanceTimersByTimeAsync(5000);
    expect(patches(d)).toEqual([]);
  });

  it("sends a monorepo's package swimlane once at load when the daemon has no row for it, and then nothing", async () => {
    const d = createFakeDaemon({
      projects: PROTOTYPE_PROJECTS,
      preferences: saved,
      savedViews: [needsMe],
    });
    daemon = d;
    const { M } = await synced(d);
    vi.useFakeTimers();
    await vi.advanceTimersByTimeAsync(5000);
    // The daemon's default swimlane is none, so the client's own start is sent, and only that.
    expect(patches(d)).toEqual([{ projects: { mobile: { swimlane: "package" } } }]);
    expect(d.me.preferences.projects.mobile).toMatchObject({
      swimlane: "package",
      lastView: "board",
    });
    expect(M.S.swim.mobile).toBe("package");
    expect(M.S.savedView.mobile).toBe("All cards");
  });

  it("sends nothing at load for a project that is not a monorepo, whether or not the daemon has a row", async () => {
    const d = createFakeDaemon({
      projects: PROTOTYPE_PROJECTS.slice(0, 2),
      preferences: saved,
      savedViews: [needsMe],
    });
    daemon = d;
    await synced(d);
    vi.useFakeTimers();
    await vi.advanceTimersByTimeAsync(5000);
    expect(patches(d)).toEqual([]);
  });

  it("starts with nothing saved as the store starts, and sends nothing", async () => {
    const d = open();
    const { M } = await synced(d);
    vi.useFakeTimers();
    await vi.advanceTimersByTimeAsync(5000);
    expect(M.S.theme).toBe("system");
    expect(patches(d)).toEqual([]);
  });
});

describe("me.updated", () => {
  const update = (d: FakeDaemon, preferences: Preferences) =>
    d.emit("me", "me.updated", {
      profile: d.me.profile,
      preferences,
      progress: d.me.progress,
    });

  it("replaces the preferences, and applies the theme to the page", async () => {
    const d = open();
    const { M, applyTheme } = await synced(d);
    applyTheme.mockClear();
    update(d, { ...emptyPreferences(), theme: "dark", listColumns: { pkg: true } });
    expect(M.S.theme).toBe("dark");
    expect(M.S.listCols.pkg).toBe(true);
    expect(applyTheme).toHaveBeenCalledTimes(1);
  });

  it("changes nothing the second time it is applied, and does not save what it brought", async () => {
    const d = open();
    const { M, applyTheme } = await synced(d);
    const next = { ...emptyPreferences(), theme: "light" as const };
    update(d, next);
    applyTheme.mockClear();
    update(d, next);
    expect(M.S.theme).toBe("light");
    expect(applyTheme).not.toHaveBeenCalled();
    vi.useFakeTimers();
    await vi.advanceTimersByTimeAsync(5000);
    expect(patches(d)).toEqual([]);
  });

  it("ignores an event of another kind and a payload without preferences", async () => {
    const d = open();
    const { M } = await synced(d);
    d.emit("me", "me.updated", { profile: d.me.profile });
    d.emit("me", "something.else", { preferences: { ...emptyPreferences(), theme: "dark" } });
    expect(M.S.theme).toBe("system");
  });

  it("draws nothing for a project the store does not have", async () => {
    const d = open();
    const { M } = await synced(d);
    const prefs = emptyPreferences();
    prefs.projects.late = { ...saved.projects.web!, lastView: "list", swimlane: "agent" };
    update(d, prefs);
    expect(M.S.lastView.late).toBeUndefined();
    expect(M.S.swim.late).toBeUndefined();
  });
});

describe("a project that is removed", () => {
  it("takes its preferences with it, and sends nothing for it", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    const { M } = await synced(d);
    d.emit("home", "project.removed", { projectId: "api" });
    expect(M.S.projects.map((project) => project.id)).not.toContain("api");
    expect(M.S.filters.api).toBeUndefined();
    expect(M.S.swim.api).toBeUndefined();
    expect(M.S.laneCollapsed["api:role:Tester"]).toBeUndefined();
    vi.useFakeTimers();
    await vi.advanceTimersByTimeAsync(5000);
    expect(patches(d)).toEqual([]);
  });

  it("starts as a new project does when it is added again under the same id", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    const { M } = await synced(d);
    d.emit("home", "project.removed", { projectId: "api" });
    d.emit("home", "project.created", {
      project: wireProject({ id: "api", name: "api", path: "/x" }),
    });
    expect(M.S.filters.api).toEqual([]);
    expect(M.S.swim.api).toBe("none");
    expect(M.S.lastView.api).toBe("board");
    vi.useFakeTimers();
    await vi.advanceTimersByTimeAsync(5000);
    expect(patches(d)).toEqual([]);
  });

  it("can be forgotten by a caller that already knows, with no daemon", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    const { M, ctx } = await synced(d);
    forgetProjectPrefs(ctx, "web");
    expect(M.S.query.web).toBeUndefined();
    noteSavedViewGone(ctx, "nowhere");
  });
});

describe("when the preferences cannot be loaded", () => {
  it("shows the daemon's sentence in place of the app, and loads them again on request", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    d.refuseNext(
      "GET /v1/me/preferences",
      500,
      "internal",
      "Marshal ran into a problem. Try again.",
    );
    const M = createTestMarshal({ data: d.data, sections: DAEMON_PERSON });
    await d.connect();
    await vi.waitFor(() => expect(M.S.loadError).toBe("Marshal ran into a problem. Try again."));
    expect(M.S.ready).toBe(false);
    M.reconnect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    expect(M.S.loadError).toBe("");
    expect(M.S.theme).toBe("dark");
  });
});
