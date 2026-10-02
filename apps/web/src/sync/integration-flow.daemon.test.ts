// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (S5a is the daemon's).
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { daemon, resetDaemonCards, resetStoreCards } from "~/testing/daemon-cards-store";
import { M } from "~/mock";
import { historyItem, idleFlow, queueItem } from "~/testing/fake-merge-flow";
import { contextOf } from "~/testing/test-store";
import {
  openCardWorktree,
  pauseMerging,
  resumeMerging,
  retryMerge,
  undoMerge,
} from "./integration-flow";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const READ = "GET /v1/projects/api/integration";
const idOf = (key: string): string => {
  const card = daemon.cards.find((one) => one.key === key);
  if (!card) throw new Error(`the fake daemon has no card ${key}`);
  return card.id;
};
const reads = (): number => daemon.routes().filter((route) => route === READ).length;
const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);
const flowOf = (pid = "api") => M.S.integration?.[pid]?.flow ?? null;

/** Gives a project a state to read: the idle one, with whatever the test says is different. */
function seed(fields: Partial<ReturnType<typeof idleFlow>> = {}, pid = "api"): void {
  daemon.mergeFlow.states[pid] = { ...idleFlow(pid, new Date().toISOString()), ...fields };
}

/** Opens the Integration view of a project, which is what makes the store read its flow. */
const show = (pid = "api") => M.go("project", pid, "integration");
const leave = () => M.go("project", "api", "board");

const inTheQueue = (
  key: string,
  phase: "queued" | "resolving" | "testing" | "landing" = "queued",
) => queueItem({ cardId: idOf(key), key, phase });

beforeEach(() => {
  for (const pid of Object.keys(daemon.mergeFlow.states)) delete daemon.mergeFlow.states[pid];
  daemon.mergeFlow.absent = false;
  daemon.mergeFlow.opened.length = 0;
  resetDaemonCards();
  resetStoreCards();
  leave();
  M.set({ integration: {}, toasts: [] });
  daemon.calls.length = 0;
});
afterEach(() => {
  vi.restoreAllMocks();
});

describe("reading a project's merge flow", () => {
  it("reads it when the Integration view is shown, and again each time it is shown", async () => {
    seed({ aheadBy: 2, state: "merging", queue: [inTheQueue("api#41", "resolving")] });
    expect(reads()).toBe(0);
    show();
    await vi.waitFor(() => expect(flowOf()?.lanes.resolving.map((c) => c.key)).toEqual(["api#41"]));
    expect(flowOf()).toMatchObject({ aheadBy: 2, state: "merging", target: "main" });
    expect(reads()).toBe(1);
    leave();
    show();
    await vi.waitFor(() => expect(reads()).toBe(2));
  });

  it("reads the flow of the project that is shown, in a split pane too", async () => {
    M.go("project", "api", "board");
    M.S.split = ["integration"];
    await vi.waitFor(() => expect(flowOf()).not.toBeNull());
    expect(reads()).toBe(1);
    M.S.split = [];
  });

  it("says nothing waits for a daemon that has no merge queue, and shows no error", async () => {
    daemon.mergeFlow.absent = true;
    show();
    await vi.waitFor(() => expect(M.S.integration?.api?.loading).toBe(false));
    expect(M.S.integration?.api).toEqual({ flow: null, error: "", loading: false });
    expect(toasts()).toEqual([]);
  });

  it("keeps what it had and says why when a later read fails", async () => {
    seed({ aheadBy: 1 });
    show();
    await vi.waitFor(() => expect(flowOf()?.aheadBy).toBe(1));
    daemon.refuseNext(READ, 503, "unavailable", "The merge queue is busy. Try again.");
    daemon.emit("project:api", "merge.progress", { projectId: "api", cardId: idOf("api#41") });
    await vi.waitFor(() =>
      expect(M.S.integration?.api?.error).toBe("The merge queue is busy. Try again."),
    );
    expect(flowOf()?.aheadBy).toBe(1);
    daemon.emit("project:api", "merge.progress", { projectId: "api", cardId: idOf("api#41") });
    await vi.waitFor(() => expect(M.S.integration?.api?.error).toBe(""));
  });
});

describe("keeping the shown flow current", () => {
  it.each([
    ["merge.progress", () => ({ projectId: "api", cardId: idOf("api#41"), phase: "testing" })],
    ["card.moved", () => ({ card: { ...daemon.cards[0], projectId: "api" }, from: "review" })],
    ["card.updated", () => ({ card: { ...daemon.cards[0], projectId: "api" } })],
  ])("reads it again on %s of the project", async (type, data) => {
    seed();
    show();
    await vi.waitFor(() => expect(flowOf()).not.toBeNull());
    seed({ state: "merging", queue: [inTheQueue("api#41", "testing")] });
    daemon.emit("project:api", type, data());
    await vi.waitFor(() => expect(flowOf()?.lanes.testing.map((c) => c.key)).toEqual(["api#41"]));
    expect(reads()).toBe(2);
  });

  it("ignores events of another project and events that are not about the merge", async () => {
    seed();
    show();
    await vi.waitFor(() => expect(flowOf()).not.toBeNull());
    daemon.emit("project:web", "merge.progress", { projectId: "web", cardId: idOf("api#41") });
    daemon.emit("project:api", "chat.updated", { projectId: "api" });
    daemon.emit("home", "ci.updated", {
      snapshot: { projects: [], serverTime: new Date().toISOString() },
    });
    await vi.waitFor(() => expect(daemon.calls.length).toBeGreaterThan(0));
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(reads()).toBe(1);
  });

  it("does not read while the view is not shown, and catches up when it is", async () => {
    seed();
    show();
    await vi.waitFor(() => expect(reads()).toBe(1));
    leave();
    seed({ aheadBy: 5 });
    daemon.emit("project:api", "merge.progress", { projectId: "api", cardId: idOf("api#41") });
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(reads()).toBe(1);
    show();
    await vi.waitFor(() => expect(flowOf()?.aheadBy).toBe(5));
  });

  it("asks for one more read, not many, while a read is on its way", async () => {
    seed();
    show();
    await vi.waitFor(() => expect(reads()).toBe(1));
    const release = daemon.holdNext(READ);
    const ping = () =>
      daemon.emit("project:api", "card.updated", {
        card: { ...daemon.cards[0], projectId: "api" },
      });
    ping();
    await vi.waitFor(() => expect(reads()).toBe(2));
    for (let i = 0; i < 9; i += 1) ping();
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(reads()).toBe(2);
    seed({ aheadBy: 3 });
    release();
    await vi.waitFor(() => expect(flowOf()?.aheadBy).toBe(3));
    expect(reads()).toBe(3);
  });

  it("does not redraw for an answer that says nothing new", async () => {
    seed({ aheadBy: 1 });
    show();
    await vi.waitFor(() => expect(flowOf()?.aheadBy).toBe(1));
    const before = M.S.integration?.api;
    daemon.emit("project:api", "card.updated", { card: { ...daemon.cards[0], projectId: "api" } });
    await vi.waitFor(() => expect(reads()).toBe(2));
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(M.S.integration?.api).toBe(before);
  });

  it("does not let a read that was on its way overwrite the state a pause just answered", async () => {
    seed();
    show();
    await vi.waitFor(() => expect(flowOf()?.state).toBe("idle"));
    let answer: (state: unknown) => void = () => undefined;
    const stale = new Promise((resolve) => {
      answer = resolve;
    });
    const api = daemon.data.api;
    const real = api.integrationState.bind(api);
    vi.spyOn(api, "integrationState").mockImplementationOnce(() => stale as never);
    vi.spyOn(api, "integrationState").mockImplementation(real);
    daemon.emit("project:api", "merge.progress", { projectId: "api", cardId: idOf("api#41") });
    await vi.waitFor(() => expect(api.integrationState).toHaveBeenCalledTimes(1));
    expect(await pauseMerging("api")).toBe(true);
    expect(flowOf()?.state).toBe("paused");
    answer({ ...idleFlow("api", new Date().toISOString()) });
    await vi.waitFor(() => expect(api.integrationState).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(daemon.mergeFlow.states.api?.state).toBe("paused"));
    expect(flowOf()?.state).toBe("paused");
  });
});

describe("pausing and resuming", () => {
  it("draws the state the daemon answers", async () => {
    seed({ queue: [inTheQueue("api#41")], state: "merging" });
    show();
    await vi.waitFor(() => expect(flowOf()?.state).toBe("merging"));
    expect(await pauseMerging("api")).toBe(true);
    expect(flowOf()).toMatchObject({ state: "paused", stateLabel: "Paused" });
    expect(daemon.routes()).toContain("POST /v1/projects/api/integration/pause");
    expect(await resumeMerging("api")).toBe(true);
    expect(flowOf()?.state).toBe("merging");
    expect(daemon.routes()).toContain("POST /v1/projects/api/integration/resume");
  });

  it("shows the daemon's own sentence when it refuses", async () => {
    daemon.refuseNext(
      "POST /v1/projects/api/integration/pause",
      422,
      "refused",
      "Nothing to pause.",
    );
    expect(await pauseMerging("api")).toBe(false);
    expect(toasts()).toContain("Nothing to pause.");
  });
});

describe("retrying and undoing a card's merge", () => {
  it("retries by the card's key or by the daemon's id, and draws the card it answers", async () => {
    const id = idOf("mobile#210");
    expect(M.S.cards.find((card) => card.id === "mobile#210")?.state).toBe("needs");
    expect(await retryMerge("mobile#210")).toBe(true);
    expect(daemon.routes()).toContain(`POST /v1/cards/${id}/merge/retry`);
    expect(M.S.cards.find((card) => card.id === "mobile#210")).toMatchObject({
      state: "merging",
      mergePhase: "queued",
    });
    expect(toasts()).toContain("Retrying the merge of mobile#210");
    expect(await retryMerge(id)).toBe(true);
    expect(daemon.routes().filter((route) => route.endsWith("/merge/retry"))).toHaveLength(2);
  });

  it("asks nothing for a key the store does not have", async () => {
    expect(await retryMerge("nope#1")).toBe(false);
    expect(await undoMerge("nope#1")).toBe(false);
    expect(await openCardWorktree("nope#1", "finder")).toBe(false);
    expect(
      daemon.routes().some((route) => route.includes("/merge/") || route.includes("/worktree/")),
    ).toBe(false);
  });

  it("undoes a delivered card, which is Ready to merge again", async () => {
    const id = idOf("api#41");
    seed({ history: [historyItem({ cardId: id, key: "api#41", canUndo: true })] });
    show();
    await vi.waitFor(() => expect(flowOf()?.delivered).toHaveLength(1));
    expect(await undoMerge("api#41")).toBe(true);
    expect(daemon.routes()).toContain(`POST /v1/cards/${id}/merge/undo`);
    expect(toasts()).toContain("Undid the merge of api#41");
    await vi.waitFor(() => expect(flowOf()?.delivered).toHaveLength(0));
  });

  it("shows the daemon's own sentence when it refuses, and leaves the card as it was", async () => {
    const id = idOf("mobile#210");
    const sentence = "That merge has already been delivered, so there is nothing to retry.";
    daemon.refuseNext(`POST /v1/cards/${id}/merge/retry`, 422, "refused", sentence);
    expect(await retryMerge("mobile#210")).toBe(false);
    expect(toasts()).toEqual([sentence]);
    expect(M.S.cards.find((card) => card.id === "mobile#210")?.state).toBe("needs");
  });

  it("refuses a second press while the first is still on its way", async () => {
    const id = idOf("mobile#210");
    const release = daemon.holdNext(`POST /v1/cards/${id}/merge/retry`);
    const first = retryMerge("mobile#210");
    expect(await retryMerge("mobile#210")).toBe(false);
    expect(toasts()).toEqual(["That change is still being saved. Wait a moment and try again."]);
    release();
    expect(await first).toBe(true);
  });
});

describe("showing a card's worktree", () => {
  it("asks the daemon to open it the way the caller chose", async () => {
    const id = idOf("api#41");
    expect(await openCardWorktree("api#41", "finder")).toBe(true);
    expect(await openCardWorktree(id, "editor")).toBe(true);
    expect(daemon.bodies(`POST /v1/cards/${id}/worktree/open`)).toEqual([
      { with: "finder" },
      { with: "editor" },
    ]);
    expect(daemon.mergeFlow.opened).toEqual([
      { cardId: id, with: "finder" },
      { cardId: id, with: "editor" },
    ]);
  });

  it("shows the daemon's own sentence when it refuses, such as a request from a phone", async () => {
    const id = idOf("api#41");
    const sentence = "Marshal opens a folder only on the computer it runs on.";
    daemon.refuseNext(`POST /v1/cards/${id}/worktree/open`, 403, "forbidden", sentence);
    expect(await openCardWorktree("api#41", "finder")).toBe(false);
    expect(toasts()).toEqual([sentence]);
  });
});

describe("when the store stops following the daemon", () => {
  it("has nothing to act on, and the actions say so", async () => {
    contextOf(M).sync?.stop();
    expect(await retryMerge("mobile#210")).toBe(false);
    expect(await pauseMerging("api")).toBe(false);
  });
});
