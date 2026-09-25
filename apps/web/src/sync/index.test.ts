import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import { TOKEN_KEY } from "~/data/token";
import type { Marshal } from "~/mock";
import { createFakeDaemon, FAKE_TOKEN, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS, wireProject } from "~/testing/projects";
import { contextOf, createSyncedMarshal, createTestMarshal } from "~/testing/test-store";
import { startSync } from ".";

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

const open = (options?: Parameters<typeof createFakeDaemon>[0]): FakeDaemon => {
  daemon = createFakeDaemon(options);
  return daemon;
};
const projectIds = (M: Marshal) => M.S.projects.map((p) => p.id);
const listCalls = (d: FakeDaemon) => d.routes().filter((r) => r === "GET /v1/projects").length;

describe("startSync with a daemon", () => {
  it("is not ready until the daemon has answered, then loads the projects and is", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS });
    const M = createTestMarshal({ data: d.data });
    expect(M.S.ready).toBe(false);
    expect(M.S.projects).toEqual([]);
    expect(M.S.connection?.state).toBe("starting");
    await d.connect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    expect(projectIds(M)).toEqual(["api", "web", "mobile"]);
    expect(M.S.cards).toHaveLength(29);
    expect(M.S.connection).toMatchObject({ state: "online", rejection: "", busy: false });
    expect(M.S.loadError).toBe("");
  });

  it('counts time from the daemon\'s clock, so "4 min ago" is right when the two clocks differ', async () => {
    const d = open({ clockSkewMs: 120_000 });
    const M = await createSyncedMarshal(d);
    expect(M.now() - Date.now()).toBeGreaterThan(110_000);
    expect(M.now() - Date.now()).toBeLessThan(130_000);
  });

  it("subscribes to the Home topic", async () => {
    const d = open();
    await createSyncedMarshal(d);
    const hello = d.sockets.last().hellos()[0];
    expect(hello?.subscribe).toEqual(["home"]);
  });

  it("applies project events from the stream, once each", async () => {
    const d = open();
    const M = await createSyncedMarshal(d);
    const project = wireProject({ id: "billing", name: "billing", path: "/code/billing" });
    d.emit("home", "project.created", { project });
    d.emit("home", "project.created", { project });
    expect(projectIds(M)).toEqual(["billing"]);
    d.emit("home", "project.updated", { project: { ...project, name: "Billing" } });
    expect(M.proj("billing")?.name).toBe("Billing");
    d.emit("home", "project.removed", { projectId: "billing" });
    expect(projectIds(M)).toEqual([]);
  });

  it("loads the snapshots again when the stream says events were missed", async () => {
    const d = open();
    const M = await createSyncedMarshal(d);
    d.projects.push(wireProject({ id: "late", name: "late", path: "/code/late" }));
    d.sockets.last().push({
      type: "resync",
      epoch: "01M3C0ZZZZ000000000000000C",
      reason: "epoch-changed",
      seq: 0,
    });
    await vi.waitFor(() => expect(projectIds(M)).toEqual(["late"]));
  });

  it("loads them again when the stream comes back after a drop, and the app never leaves the screen", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS });
    const M = await createSyncedMarshal(d);
    const before = listCalls(d);
    d.sockets.last().drop();
    await vi.waitFor(() => expect(M.S.connection?.state).toBe("reconnecting"));
    expect(M.S.ready).toBe(true);
    d.projects.pop();
    await vi.waitFor(() => expect(d.sockets.all.length).toBeGreaterThan(1), { timeout: 3000 });
    d.sockets.last().accept();
    await vi.waitFor(() => expect(M.S.connection?.state).toBe("online"));
    await vi.waitFor(() => expect(projectIds(M)).toEqual(["api", "web"]));
    expect(listCalls(d)).toBeGreaterThan(before);
  });

  it("is ready when the daemon cannot be reached, so the full screen can draw, and recovers by itself", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS });
    d.stop();
    const M = createTestMarshal({ data: d.data });
    await vi.waitFor(() => expect(M.S.connection?.state).toBe("unreachable"));
    expect(M.S.ready).toBe(true);
    expect(M.S.connection?.detail).toContain("unreachable");
    expect(M.S.connection?.retryAt).toEqual(expect.any(Number));
    expect(M.S.projects).toEqual([]);
    d.start();
    M.reconnect();
    await d.connect();
    await vi.waitFor(() => expect(projectIds(M)).toEqual(["api", "web", "mobile"]));
    expect(M.S.connection?.state).toBe("online");
    expect(M.S.connection?.retryAt).toBeNull();
  });

  it("shows a load error, not the app, when the daemon answers but the projects cannot be loaded, and tries again on request", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS });
    d.refuseNext("GET /v1/projects", 500, "internal", "Marshal ran into a problem. Try again.");
    const M = createTestMarshal({ data: d.data });
    await d.connect();
    await vi.waitFor(() => expect(M.S.loadError).toBe("Marshal ran into a problem. Try again."));
    expect(M.S.ready).toBe(false);
    M.reconnect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    expect(M.S.loadError).toBe("");
    expect(projectIds(M)).toHaveLength(3);
  });
});

describe("sign in", () => {
  it("shows no rejection to a device that has no token yet", async () => {
    const d = open({ storedToken: null });
    const M = createTestMarshal({ data: d.data });
    await vi.waitFor(() => expect(M.S.connection?.state).toBe("unauthorized"));
    expect(M.S.ready).toBe(true);
    expect(M.S.connection?.rejection).toBe("");
  });

  it("says plainly when the token that was entered is refused, and keeps asking", async () => {
    const d = open({ storedToken: null });
    const M = createTestMarshal({ data: d.data });
    await vi.waitFor(() => expect(M.S.connection?.state).toBe("unauthorized"));
    M.signIn("  not-the-token ");
    expect(M.S.connection?.busy).toBe(true);
    await vi.waitFor(() => expect(M.S.connection?.rejection).not.toBe(""));
    expect(M.S.connection?.rejection).toBe(
      "Sign in again. This device's token is missing or no longer valid.",
    );
    expect(M.S.connection?.busy).toBe(false);
    expect(M.S.connection?.state).toBe("unauthorized");
    // The token is trimmed and kept by the token store only.
    expect(d.storage.values.get(TOKEN_KEY)).toBe("not-the-token");
    expect(JSON.stringify(M.S)).not.toContain("not-the-token");
  });

  it("loads the app once the right token is entered", async () => {
    const d = open({ storedToken: null, projects: PROTOTYPE_PROJECTS });
    const M = createTestMarshal({ data: d.data });
    await vi.waitFor(() => expect(M.S.connection?.state).toBe("unauthorized"));
    M.signIn(FAKE_TOKEN);
    await d.connect();
    await vi.waitFor(() => expect(projectIds(M)).toEqual(["api", "web", "mobile"]));
    expect(M.S.connection?.state).toBe("online");
    expect(d.storage.values.get(TOKEN_KEY)).toBe(FAKE_TOKEN);
  });

  it("ignores an empty token", async () => {
    const d = open({ storedToken: null });
    const M = createTestMarshal({ data: d.data });
    await vi.waitFor(() => expect(M.S.connection?.state).toBe("unauthorized"));
    M.signIn("   ");
    expect(M.S.connection?.busy).toBe(false);
    expect(d.storage.writes).toEqual([]);
  });
});

describe("startSync without one", () => {
  it("does nothing: there is no connection, no fetch, and the store is left as it is", () => {
    const M = createTestMarshal();
    expect(startSync(contextOf(M))).toBeNull();
    expect(M.S.connection).toBeUndefined();
    expect(M.S.loadError).toBeUndefined();
  });

  it("skips a section that is still on the mock", async () => {
    const d = open({ projects: PROTOTYPE_PROJECTS });
    const M = createTestMarshal({ data: d.data, sections: { ...sectionStatus, S3: "mock" } });
    await d.connect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    expect(listCalls(d)).toBe(0);
    expect(M.S.projects).toEqual([]);
  });

  it("stops following the daemon when asked", async () => {
    const d = open();
    const M = await createSyncedMarshal(d);
    contextOf(M).sync?.stop();
    d.emit("home", "project.created", {
      project: wireProject({ id: "ignored", name: "ignored", path: "/code/ignored" }),
    });
    expect(projectIds(M)).toEqual([]);
    expect(contextOf(M).sync).toBeNull();
  });
});
