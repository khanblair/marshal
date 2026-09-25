import { afterEach, describe, expect, it, vi } from "vitest";
import type { Marshal } from "~/mock";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { createSyncedMarshal, createTestMarshal } from "~/testing/test-store";

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

async function setup(): Promise<{ M: Marshal; d: FakeDaemon }> {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS });
  daemon = d;
  return { M: await createSyncedMarshal(d), d };
}
const ids = (M: Marshal) => M.S.projects.map((p) => p.id);
const toasts = (M: Marshal) => M.S.toasts.map((t) => t.msg);
const feedLines = (M: Marshal, text: string) => M.S.feed.filter((f) => f.text.startsWith(text));

describe("addProject", () => {
  it("sends the request as it is, then shows the project once, though the event arrives too", async () => {
    const { M, d } = await setup();
    const result = await M.addProject({ source: "folder", path: "/code/billing", name: "Billing" });
    expect(result).toEqual({ id: "billing" });
    expect(d.bodies("POST /v1/projects")).toEqual([
      { source: "folder", path: "/code/billing", name: "Billing" },
    ]);
    // The fake daemon also published project.created on the stream, and it changed nothing.
    expect(ids(M)).toEqual(["api", "web", "mobile", "billing"]);
    expect(M.proj("billing")).toMatchObject({
      name: "Billing",
      path: "/code/billing",
      lang: "Unknown",
    });
    expect(M.proj("billing")?.ci).toBeUndefined();
    expect(M.cardsOf("billing")).toEqual([]);
    expect(M.S.chats.billing).toEqual([]);
    expect(toasts(M)).toEqual([]);
  });

  it("returns the daemon's sentence and changes nothing when it refuses", async () => {
    const { M, d } = await setup();
    expect(await M.addProject({ source: "folder", path: "  " })).toEqual({
      error: "Choose the folder of the repository.",
    });
    d.refuseNext(
      "POST /v1/projects",
      409,
      "conflict",
      "That repository is already a project in Marshal.",
    );
    const again = await M.addProject({ source: "folder", path: "/code/api" });
    expect(again).toEqual({ error: "That repository is already a project in Marshal." });
    expect(ids(M)).toEqual(["api", "web", "mobile"]);
    expect(toasts(M)).toEqual([]);
  });

  it("says so plainly when there is no daemon", async () => {
    const M = createTestMarshal();
    expect(await M.addProject({ source: "folder", path: "/code/x" })).toEqual({
      error: "Marshal is not connected to its daemon.",
    });
  });
});

describe("renameProject", () => {
  it("shows the new name at once, asks the daemon for only the name, and keeps its answer", async () => {
    const { M, d } = await setup();
    const release = d.holdNext("PATCH /v1/projects/api");
    const done = M.renameProject("api", "  gateway ");
    expect(M.proj("api")?.name).toBe("gateway");
    release();
    expect(await done).toBe(true);
    expect(d.bodies("PATCH /v1/projects/api")).toEqual([{ name: "gateway" }]);
    expect(M.proj("api")?.name).toBe("gateway");
    expect(toasts(M)).toEqual([]);
  });

  it("puts the old name back and shows the daemon's sentence when it says no", async () => {
    const { M, d } = await setup();
    d.refuseNext(
      "PATCH /v1/projects/api",
      400,
      "invalid_argument",
      "Project names can have at most 80 characters.",
    );
    expect(await M.renameProject("api", "x".repeat(200))).toBe(false);
    expect(M.proj("api")?.name).toBe("api-gateway");
    expect(toasts(M)).toEqual(["Project names can have at most 80 characters."]);
  });

  it("keeps the old name, without asking, when the new one is empty or the same", async () => {
    const { M, d } = await setup();
    expect(await M.renameProject("api", "   ")).toBe(false);
    expect(toasts(M)).toEqual(["Project names can't be empty. The old name is kept."]);
    expect(await M.renameProject("api", "api-gateway")).toBe(true);
    expect(await M.renameProject("nope", "x")).toBe(false);
    expect(d.routes().filter((r) => r.startsWith("PATCH"))).toEqual([]);
  });

  it("refuses a second change to the same project while one is still being saved", async () => {
    const { M, d } = await setup();
    const release = d.holdNext("PATCH /v1/projects/api");
    const first = M.renameProject("api", "one");
    expect(await M.renameProject("api", "two")).toBe(false);
    expect(toasts(M)).toEqual(["That change is still being saved. Wait a moment and try again."]);
    expect(M.proj("api")?.name).toBe("one");
    release();
    await first;
    expect(d.bodies("PATCH /v1/projects/api")).toEqual([{ name: "one" }]);
  });
});

describe("saveProject", () => {
  const same = { name: "api-gateway", branch: "main", dev: "", lockBypass: false };

  it("sends only the fields that changed, and toasts when the daemon has them", async () => {
    const { M, d } = await setup();
    expect(await M.saveProject("api", { ...same, dev: "go run .", lockBypass: true })).toBe(true);
    expect(d.bodies("PATCH /v1/projects/api")).toEqual([
      { devCommand: "go run .", bypassLocked: true },
    ]);
    expect(M.proj("api")).toMatchObject({ dev: "go run .", lockBypass: true, name: "api-gateway" });
    expect(toasts(M)).toEqual(["Project saved"]);
  });

  it("puts every field back when the daemon refuses", async () => {
    const { M, d } = await setup();
    d.refuseNext(
      "PATCH /v1/projects/api",
      400,
      "invalid_argument",
      "That branch does not exist in the repository.",
    );
    expect(
      await M.saveProject("api", { name: "Renamed", branch: "nope", dev: "x", lockBypass: true }),
    ).toBe(false);
    expect(M.proj("api")).toMatchObject({
      name: "api-gateway",
      branch: "main",
      dev: "",
      lockBypass: false,
    });
    expect(toasts(M)).toEqual(["That branch does not exist in the repository."]);
  });

  it("asks for nothing when nothing changed", async () => {
    const { M, d } = await setup();
    expect(await M.saveProject("api", same)).toBe(true);
    expect(await M.saveProject("nope", same)).toBe(false);
    expect(d.routes().filter((r) => r.startsWith("PATCH"))).toEqual([]);
  });
});

describe("removeProject", () => {
  it("sends the two choices exactly, hides the project and its mock records at once, and forgets them on success", async () => {
    const { M, d } = await setup();
    M.go("project", "web", "board");
    const release = d.holdNext("DELETE /v1/projects/web");
    const done = M.removeProject("web", { keepBranches: false, keepMemory: true });
    expect(ids(M)).toEqual(["api", "mobile"]);
    expect(M.cardsOf("web")).toEqual([]);
    expect(M.S.route.page).toBe("home");
    release();
    expect(await done).toBe(true);
    expect(d.bodies("DELETE /v1/projects/web")).toEqual([
      { keepBranches: false, keepMemory: true },
    ]);
    expect(toasts(M)).toEqual(["Project removed"]);
    // The event for the same removal arrived as well, and added no second line.
    expect(feedLines(M, "web-dashboard was removed from Marshal.")).toHaveLength(1);
    expect(feedLines(M, "web-dashboard was removed")[0]?.text).toContain(
      "The repository on disk was not touched.",
    );
  });

  it.each([
    [true, true],
    [true, false],
    [false, true],
    [false, false],
  ])(
    "sends keepBranches %s and keepMemory %s as they are, never an empty request",
    async (keepBranches, keepMemory) => {
      const { M, d } = await setup();
      await M.removeProject("api", { keepBranches, keepMemory });
      expect(d.bodies("DELETE /v1/projects/api")).toEqual([{ keepBranches, keepMemory }]);
    },
  );

  it("brings the project and its mock records back, and says why, when the daemon refuses", async () => {
    const { M, d } = await setup();
    const cards = M.cardsOf("web").length;
    d.refuseNext(
      "DELETE /v1/projects/web",
      503,
      "unavailable",
      "Marshal could not remove that project. Try again.",
    );
    expect(await M.removeProject("web", { keepBranches: true, keepMemory: true })).toBe(false);
    expect(ids(M)).toEqual(["api", "web", "mobile"]);
    expect(M.cardsOf("web")).toHaveLength(cards);
    expect(toasts(M)).toEqual(["Marshal could not remove that project. Try again."]);
    expect(feedLines(M, "web-dashboard was removed")).toHaveLength(0);
  });

  it("does nothing for a project that is not there", async () => {
    const { M, d } = await setup();
    expect(await M.removeProject("nope", { keepBranches: true, keepMemory: true })).toBe(false);
    expect(d.routes().filter((r) => r.startsWith("DELETE"))).toEqual([]);
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
  });
});
