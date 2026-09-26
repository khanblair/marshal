import type { Preferences, SavedView } from "@marshal/protocol";
import { afterEach, describe, expect, it, vi } from "vitest";
import { golden } from "~/data/testing/golden";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { withMobileRow } from "~/testing/fake-me";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import {
  contextOf,
  createSyncedMarshal,
  createTestMarshal,
  DAEMON_PERSON,
} from "~/testing/test-store";

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

/** A store on the daemon, waited for until its first snapshots are in, with the clock now the test's. */
async function synced(d: FakeDaemon) {
  const applyTheme = vi.fn();
  const M = await createSyncedMarshal(d, { sections: DAEMON_PERSON, applyTheme });
  vi.useFakeTimers();
  return { M, applyTheme, ctx: contextOf(M) };
}

const patches = (d: FakeDaemon): unknown[] => d.bodies("PATCH /v1/me/preferences");
const wait = (ms: number) => vi.advanceTimersByTimeAsync(ms);
const REFUSED = "That is not a theme Marshal knows. Choose light, dark, or system.";

describe("saving a change", () => {
  it("sends a theme change once, after a short wait, with only the theme in it", async () => {
    const d = open();
    const { M } = await synced(d);
    M.setTheme("dark");
    await wait(100);
    expect(patches(d)).toEqual([]);
    await wait(100);
    expect(patches(d)).toEqual([{ theme: "dark" }]);
    expect(d.me.preferences.theme).toBe("dark");
  });

  it("does not send the daemon's own echo back, and does not change what the person sees", async () => {
    const d = open();
    const { M, applyTheme } = await synced(d);
    M.setTheme("dark");
    await wait(200);
    applyTheme.mockClear();
    await wait(5000);
    expect(patches(d)).toHaveLength(1);
    expect(M.S.theme).toBe("dark");
    expect(applyTheme).not.toHaveBeenCalled();
  });

  it("puts changes made within the wait into one request, and only what changed", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    const { M, ctx } = await synced(d);
    M.setTheme("light");
    M.S.listCols.upd = false;
    M.S.sort.agents = { k: "cost", dir: 1 };
    M.go("project", "web", "list");
    M.addFilter("role", "Tester");
    await wait(100);
    ctx.S.laneCollapsed["web:role:Builder"] = true;
    ctx.S.showAllDone.web = true;
    await wait(200);
    expect(patches(d)).toEqual([
      {
        theme: "light",
        listColumns: { upd: false },
        sort: { agents: { key: "cost", direction: "asc" } },
        projects: {
          web: {
            lastView: "list",
            filters: [{ key: "role", value: "Tester" }],
            collapsedLanes: ["role:Builder"],
            showAllDone: true,
          },
        },
      },
    ]);
  });

  it("saves the search text when typing pauses, and not on each key", async () => {
    const d = open();
    const { ctx } = await synced(d);
    for (const text of ["c", "ca", "cac", "cache"]) {
      ctx.S.query.api = text;
      await wait(300);
    }
    expect(patches(d)).toEqual([]);
    await wait(400);
    expect(patches(d)).toEqual([{ projects: { api: { query: "cache" } } }]);
  });

  it("does not make a theme change wait for the longer time a search needs", async () => {
    const d = open();
    const { M, ctx } = await synced(d);
    ctx.S.query.api = "cache";
    M.setTheme("dark");
    await wait(200);
    expect(patches(d)).toEqual([{ theme: "dark", projects: { api: { query: "cache" } } }]);
  });

  it("sends nothing for a change that was changed back within the wait", async () => {
    const d = open();
    const { M } = await synced(d);
    M.setTheme("dark");
    await wait(100);
    M.setTheme("system");
    await wait(5000);
    expect(patches(d)).toEqual([]);
  });

  it("says the view in use was let go with the empty string, in the same request as the filters", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    const { M } = await synced(d);
    expect(M.S.savedView.api).toBe("Needs me");
    M.removeFilter("status", "needs");
    await wait(200);
    expect(patches(d)).toEqual([{ projects: { api: { filters: [], savedViewId: "" } } }]);
    expect(d.me.preferences.projects.api?.savedViewId).toBeNull();
  });

  it("sends the view chosen from the menu with its filters and swimlane, in one request", async () => {
    const d = open({ savedViews: [needsMe] });
    const { M } = await synced(d);
    M.go("project", "api");
    M.applyView("Needs me");
    await wait(200);
    expect(patches(d)).toEqual([
      {
        projects: {
          api: {
            filters: [{ key: "status", value: "needs" }],
            savedViewId: needsMe.id,
          },
        },
      },
    ]);
  });

  it("does not send a project's preferences that are the defaults for a project it has never saved", async () => {
    const d = open();
    const { M } = await synced(d);
    M.go("project", "web", "board");
    await wait(5000);
    expect(patches(d)).toEqual([]);
  });

  it("sends a monorepo's package swimlane with the first thing saved for it, so the daemon's row keeps it", async () => {
    // The daemon has no row for `mobile`, and its own default swimlane is none.
    const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS });
    daemon = d;
    const M = await createSyncedMarshal(d, { sections: DAEMON_PERSON, applyTheme: vi.fn() });
    vi.useFakeTimers();
    M.go("project", "mobile", "list");
    await wait(200);
    expect(patches(d)).toEqual([
      { projects: { mobile: { lastView: "list", swimlane: "package" } } },
    ]);
    expect(d.me.preferences.projects.mobile?.swimlane).toBe("package");
    expect(M.S.swim.mobile).toBe("package");
    M.setView("board");
    await wait(5000);
    expect(M.S.swim.mobile).toBe("package");
    expect(d.me.preferences.projects.mobile).toMatchObject({
      lastView: "board",
      swimlane: "package",
    });
  });

  it("remembers a monorepo set back to no swimlane, which the daemon keeps once it has the row", async () => {
    const d = open();
    const { M, ctx } = await synced(d);
    expect(M.S.swim.mobile).toBe("package");
    ctx.S.swim.mobile = "none";
    await wait(200);
    expect(patches(d)).toEqual([{ projects: { mobile: { swimlane: "none" } } }]);
    expect(d.me.preferences.projects.mobile?.swimlane).toBe("none");
    await wait(5000);
    expect(patches(d)).toHaveLength(1);
    expect(M.S.swim.mobile).toBe("none");
  });
});

describe("saving in order", () => {
  it("sends a change made while one is on its way after that one is answered, never beside it", async () => {
    const d = open();
    const { M } = await synced(d);
    const release = d.holdNext("PATCH /v1/me/preferences");
    M.setTheme("dark");
    await wait(200);
    M.S.sort.list = { k: "cost", dir: 1 };
    await wait(5000);
    expect(patches(d)).toEqual([{ theme: "dark" }]);
    release();
    await wait(500);
    expect(patches(d)).toEqual([
      { theme: "dark" },
      { sort: { list: { key: "cost", direction: "asc" } } },
    ]);
    expect(d.me.preferences.theme).toBe("dark");
    expect(d.me.preferences.sort.list).toEqual({ key: "cost", direction: "asc" });
  });

  it("does not let the echo of an earlier change undo what the person changed since", async () => {
    const d = open();
    const { M } = await synced(d);
    const release = d.holdNext("PATCH /v1/me/preferences");
    M.setTheme("dark");
    await wait(200);
    M.setTheme("light");
    release();
    await wait(500);
    expect(M.S.theme).toBe("light");
    expect(patches(d)).toEqual([{ theme: "dark" }, { theme: "light" }]);
    expect(d.me.preferences.theme).toBe("light");
    await wait(5000);
    expect(patches(d)).toHaveLength(2);
    expect(M.S.theme).toBe("light");
  });

  it("does not let typing be replaced by an echo of the search before it", async () => {
    const d = open();
    const { ctx } = await synced(d);
    ctx.S.query.api = "ca";
    await wait(800);
    const release = d.holdNext("PATCH /v1/me/preferences");
    ctx.S.query.api = "cac";
    await wait(800);
    ctx.S.query.api = "cache";
    release();
    await wait(1000);
    expect(ctx.S.query.api).toBe("cache");
    expect(d.me.preferences.projects.api?.query).toBe("cache");
  });
});

describe("a change the daemon refuses", () => {
  it("shows the daemon's sentence and puts the store back to what the daemon has", async () => {
    const d = open();
    const { M } = await synced(d);
    d.refuseNext("PATCH /v1/me/preferences", 400, "invalid_argument", REFUSED);
    M.setTheme("dark");
    await wait(200);
    expect(M.S.toasts.map((toast) => toast.msg)).toEqual([REFUSED]);
    expect(M.S.theme).toBe("system");
    await wait(5000);
    expect(patches(d)).toHaveLength(1);
  });

  it("puts back a project's fields, and applies the theme to the page again", async () => {
    const d = open({ preferences: saved, savedViews: [needsMe] });
    const { M, ctx, applyTheme } = await synced(d);
    d.refuseNext(
      "PATCH /v1/me/preferences",
      400,
      "invalid_argument",
      "A search can have at most 200 characters.",
    );
    M.setTheme("light");
    ctx.S.query.api = "x".repeat(300);
    await wait(1000);
    expect(ctx.S.query.api).toBe("cache");
    expect(M.S.theme).toBe("dark");
    expect(applyTheme).toHaveBeenLastCalledWith(ctx.S);
    expect(M.S.toasts.at(-1)?.msg).toBe("A search can have at most 200 characters.");
  });

  it("lets the same change be tried again once it was put back", async () => {
    const d = open();
    const { M } = await synced(d);
    d.refuseNext(
      "PATCH /v1/me/preferences",
      500,
      "internal",
      "Marshal ran into a problem. Try again.",
    );
    M.setTheme("dark");
    await wait(200);
    expect(M.S.theme).toBe("system");
    M.setTheme("dark");
    await wait(200);
    expect(patches(d)).toEqual([{ theme: "dark" }, { theme: "dark" }]);
    expect(d.me.preferences.theme).toBe("dark");
  });

  it("does not send a refused change again when it could not be put back", async () => {
    const d = open();
    const { ctx } = await synced(d);
    // A column the store has and the daemon's defaults do not: there is nothing to put it back to.
    d.refuseNext(
      "PATCH /v1/me/preferences",
      400,
      "invalid_argument",
      "A column key is a short word.",
    );
    ctx.S.listCols.zz = true;
    await wait(200);
    expect(ctx.S.toasts.map((toast) => toast.msg)).toEqual(["A column key is a short word."]);
    await wait(5000);
    expect(patches(d)).toHaveLength(1);
    // Any other change makes a different request, and that is sent.
    ctx.S.query.api = "x";
    await wait(1000);
    expect(patches(d)).toHaveLength(2);
  });

  it("keeps what the person typed after the refused change was sent", async () => {
    const d = open();
    const { M } = await synced(d);
    const release = d.holdNext("PATCH /v1/me/preferences");
    d.refuseNext("PATCH /v1/me/preferences", 400, "invalid_argument", REFUSED);
    M.setTheme("dark");
    await wait(200);
    M.setTheme("light");
    release();
    await wait(500);
    expect(M.S.theme).toBe("light");
    expect(d.me.preferences.theme).toBe("light");
  });
});

describe("a daemon that cannot be reached", () => {
  it("keeps the change and says nothing, then sends it when the daemon is back", async () => {
    const d = open();
    const { M, ctx } = await synced(d);
    d.stop();
    M.setTheme("dark");
    await wait(200);
    expect(M.S.toasts).toEqual([]);
    expect(M.S.theme).toBe("dark");
    await wait(5000);
    d.start();
    await ctx.sync?.reload();
    await wait(500);
    expect(d.me.preferences.theme).toBe("dark");
    expect(M.S.theme).toBe("dark");
    expect(M.S.toasts).toEqual([]);
  });
});

describe("two devices", () => {
  /** A second browser on the same daemon, with its own store. */
  async function second(d: FakeDaemon) {
    const data = d.another();
    others.push(data);
    const applyTheme = vi.fn();
    const M = createTestMarshal({ data, sections: DAEMON_PERSON, applyTheme });
    vi.useRealTimers();
    await d.connectAnother(data);
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    vi.useFakeTimers();
    return { M, applyTheme, ctx: contextOf(M) };
  }

  it("shows a change made on one on the other, and the other sends nothing back", async () => {
    const d = open({ savedViews: [needsMe] });
    const a = await synced(d);
    const b = await second(d);
    a.M.setTheme("dark");
    a.ctx.S.lastView.web = "list";
    a.ctx.S.filters.api = [{ k: "status", v: "needs" }];
    await wait(300);
    expect(b.M.S.theme).toBe("dark");
    expect(b.applyTheme).toHaveBeenCalled();
    expect(b.M.S.lastView.web).toBe("list");
    expect(b.M.S.filters.api).toEqual([{ k: "status", v: "needs" }]);
    await wait(5000);
    expect(patches(d)).toHaveLength(1);
  });

  it("converges when both change different things at once", async () => {
    const d = open();
    const a = await synced(d);
    const b = await second(d);
    a.M.setTheme("dark");
    b.ctx.S.listCols.upd = false;
    await wait(1000);
    for (const side of [a, b]) {
      expect(side.M.S.theme).toBe("dark");
      expect(side.M.S.listCols.upd).toBe(false);
    }
    expect(d.me.preferences.theme).toBe("dark");
    expect(d.me.preferences.listColumns).toEqual({ upd: false });
  });

  it("ends with the same value on both when they change the same thing, the last one saved winning", async () => {
    const d = open();
    const a = await synced(d);
    const b = await second(d);
    a.M.setTheme("dark");
    await wait(200);
    b.M.setTheme("light");
    await wait(1000);
    expect(a.M.S.theme).toBe(b.M.S.theme);
    expect(d.me.preferences.theme).toBe(b.M.S.theme);
  });
});
