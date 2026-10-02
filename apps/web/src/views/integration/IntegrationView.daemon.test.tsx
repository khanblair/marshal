// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (S5a is the daemon's).
import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { daemon, resetDaemonCards, resetStoreCards } from "~/testing/daemon-cards-store";
import { M } from "~/mock";
import { historyItem, idleFlow, queueItem } from "~/testing/fake-merge-flow";
import { IntegrationView } from "./IntegrationView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * The Integration view against a store whose cards are the daemon's: what the Integrator is merging
 * and what it delivered are read when the view is shown, and kept current by the events of the
 * project. The cards are the prototype's, so a queue item that names one is drawn from the store and
 * its stopped merge (mobile#210, a conflict) is in the Needs you lane without anyone seeding it.
 */
const READ = (pid: string) => `GET /v1/projects/${pid}/integration`;
const idOf = (key: string): string => {
  const card = daemon.cards.find((one) => one.key === key);
  if (!card) throw new Error(`the fake daemon has no card ${key}`);
  return card.id;
};
const titleOf = (key: string): string => daemon.cards.find((one) => one.key === key)?.title ?? "";
const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);
const count = (route: string): number => daemon.routes().filter((one) => one === route).length;

function seed(pid: string, fields: Partial<ReturnType<typeof idleFlow>> = {}): void {
  daemon.mergeFlow.states[pid] = { ...idleFlow(pid, new Date().toISOString()), ...fields };
}

/** Opens the project's Integration view, which is what makes the store read its flow. */
function show(pid: string) {
  M.go("project", pid, "integration");
  return render(() => <IntegrationView />);
}

const region = (name: string) => screen.getByRole("region", { name });
const press = (name: string, inside: HTMLElement | undefined = undefined): void => {
  fireEvent.click(
    inside ? within(inside).getByRole("button", { name }) : screen.getByRole("button", { name }),
  );
};
const heading = () => screen.findByRole("heading", { level: 2 });

beforeEach(() => {
  for (const pid of Object.keys(daemon.mergeFlow.states)) delete daemon.mergeFlow.states[pid];
  daemon.mergeFlow.absent = false;
  daemon.mergeFlow.opened.length = 0;
  resetDaemonCards();
  resetStoreCards();
  M.go("project", "api", "board");
  M.set({ integration: {}, toasts: [] });
  daemon.calls.length = 0;
});
afterEach(cleanup);

describe("an Integrator with nothing to do", () => {
  it("shows where cards land, how far ahead the Integrator is, and that nothing waits", async () => {
    show("api");
    expect(await heading()).toHaveTextContent("Merging into main");
    expect(screen.getByText("integrator")).toBeInTheDocument();
    expect(screen.getByText("Up to date")).toBeInTheDocument();
    expect(screen.getByText("Idle")).toBeInTheDocument();
    expect(screen.getByText("Nothing is waiting to merge.")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Delivered" })).toBeNull();
    expect(screen.getByRole("button", { name: "Pause merging" })).toBeInTheDocument();
    expect(count(READ("api"))).toBe(1);
  });

  it("shows the same empty state for a daemon that has no merge queue, with no error", async () => {
    daemon.mergeFlow.absent = true;
    show("api");
    await vi.waitFor(() => expect(count(READ("api"))).toBe(1));
    expect(await screen.findByText("Nothing is waiting to merge.")).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
    expect(toasts()).toEqual([]);
  });

  it("offers Try again when the first read fails, and draws the answer of the next", async () => {
    daemon.refuseNext(READ("api"), 503, "unavailable", "The merge queue is busy. Try again.");
    show("api");
    expect(await screen.findByText("The merge queue is busy. Try again.")).toBeInTheDocument();
    press("Try again");
    expect(await heading()).toHaveTextContent("Merging into main");
    expect(screen.queryByText("The merge queue is busy. Try again.")).toBeNull();
  });
});

describe("an Integrator that is merging a queue", () => {
  beforeEach(() => {
    seed("api", {
      state: "merging",
      aheadBy: 2,
      currentCardId: idOf("api#41"),
      queue: [
        queueItem({
          cardId: idOf("api#41"),
          key: "api#41",
          title: titleOf("api#41"),
          phase: "resolving",
          position: 1,
        }),
        queueItem({
          cardId: idOf("api#42"),
          key: "api#42",
          title: titleOf("api#42"),
          phase: "queued",
          position: 2,
        }),
        queueItem({
          cardId: idOf("api#44"),
          key: "api#44",
          title: titleOf("api#44"),
          phase: "testing",
          position: 3,
        }),
      ],
      history: [
        historyItem({
          cardId: idOf("api#43"),
          key: "api#43",
          title: titleOf("api#43"),
          resolved: 2,
          summary: "Both cards edited the token store; kept both.",
          canUndo: true,
        }),
      ],
    });
  });

  it("puts each card in the lane of its phase, named as the board names it", async () => {
    show("api");
    expect(await screen.findByText("Merging api#41")).toBeInTheDocument();
    expect(screen.getByText("Ahead by 2")).toBeInTheDocument();
    const resolving = region("Resolving conflicts");
    expect(within(resolving).getByRole("button", { name: titleOf("api#41") })).toBeInTheDocument();
    expect(within(resolving).getByText("api#41")).toBeInTheDocument();
    expect(within(region("Waiting to merge")).getByText("Place 2 in line")).toBeInTheDocument();
    expect(within(region("Testing")).getByText("api#44")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Landing in your folder" })).toBeNull();
    expect(screen.queryByText("Nothing is waiting to merge.")).toBeNull();
  });

  it("opens the card from its name", async () => {
    show("api");
    await screen.findByText("Merging api#41");
    fireEvent.click(
      within(region("Waiting to merge")).getByRole("button", { name: titleOf("api#42") }),
    );
    expect(M.S.openId).toBe("api#42");
  });

  it("lists what was delivered with when, what was resolved, and the Integrator's own words", async () => {
    show("api");
    const delivered = await screen.findByRole("region", { name: "Delivered" });
    const row = within(delivered).getByRole("article", { name: /^api#43 / });
    expect(within(row).getByText("api#43")).toBeInTheDocument();
    expect(row.querySelector("time")).not.toBeNull();
    expect(within(row).getByText("Resolved 2 conflicts")).toBeInTheDocument();
    expect(
      within(row).getByText("Both cards edited the token store; kept both."),
    ).toBeInTheDocument();
    expect(within(row).getByRole("button", { name: "Undo" })).toBeInTheDocument();
  });

  it("follows the merge as events arrive for its project", async () => {
    show("api");
    await screen.findByText("Merging api#41");
    seed("api", {
      state: "merging",
      currentCardId: idOf("api#42"),
      queue: [queueItem({ cardId: idOf("api#42"), key: "api#42", phase: "landing", position: 1 })],
    });
    daemon.emit("project:api", "merge.progress", {
      projectId: "api",
      cardId: idOf("api#42"),
      phase: "landing",
    });
    expect(await screen.findByText("Merging api#42")).toBeInTheDocument();
    expect(region("Landing in your folder")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Resolving conflicts" })).toBeNull();
  });

  it("does not read the flow while another view of the project is shown", async () => {
    show("api");
    await screen.findByText("Merging api#41");
    cleanup();
    M.go("project", "api", "board");
    daemon.emit("project:api", "merge.progress", { projectId: "api", cardId: idOf("api#41") });
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(count(READ("api"))).toBe(1);
  });
});

describe("Pause and Resume", () => {
  it("pauses merging, asks the daemon, and offers Resume", async () => {
    show("api");
    await heading();
    press("Pause merging");
    expect(await screen.findByRole("button", { name: "Resume merging" })).toBeInTheDocument();
    expect(daemon.routes()).toContain("POST /v1/projects/api/integration/pause");
    expect(screen.getByText("Paused")).toBeInTheDocument();
  });

  it("resumes a paused Integrator", async () => {
    seed("api", { state: "paused", queue: [queueItem({ cardId: idOf("api#41"), key: "api#41" })] });
    show("api");
    expect(await screen.findByText("Paused")).toBeInTheDocument();
    press("Resume merging");
    expect(await screen.findByRole("button", { name: "Pause merging" })).toBeInTheDocument();
    expect(daemon.routes()).toContain("POST /v1/projects/api/integration/resume");
    expect(screen.getByText("Merging api#41")).toBeInTheDocument();
  });

  it("shows the daemon's own sentence when it refuses, and keeps the button", async () => {
    daemon.refuseNext(
      "POST /v1/projects/api/integration/pause",
      422,
      "refused",
      "Merging cannot be paused now.",
    );
    show("api");
    await heading();
    press("Pause merging");
    await vi.waitFor(() => expect(toasts()).toEqual(["Merging cannot be paused now."]));
    expect(screen.getByRole("button", { name: "Pause merging" })).toBeEnabled();
  });
});

describe("a merge that stopped", () => {
  const stopped = {
    state: "waiting" as const,
    message: "queue.ts changed on both sides and the Integrator is not sure which to keep.",
  };

  it("says why it stopped and lists the card with its reason and a Retry", async () => {
    seed("mobile", { ...stopped, currentCardId: idOf("mobile#210") });
    show("mobile");
    expect(await screen.findByText("Waiting for you")).toBeInTheDocument();
    expect(screen.getByText(stopped.message)).toBeInTheDocument();
    const needs = region("Needs you");
    const card = within(needs).getByRole("article", { name: /^mobile#210 / });
    expect(
      within(card).getByText("Merge conflict: queue.ts also changed by #208"),
    ).toBeInTheDocument();
    expect(within(card).getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("lists a merge conflict in Needs you even while the Integrator is idle", async () => {
    show("mobile");
    expect(await screen.findByText("Idle")).toBeInTheDocument();
    expect(
      within(region("Needs you")).getByRole("article", { name: /^mobile#210 / }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Nothing is waiting to merge.")).toBeNull();
  });

  it("leaves a card that waits for another reason out of Needs you", async () => {
    show("api");
    await heading();
    expect(screen.queryByRole("region", { name: "Needs you" })).toBeNull();
    expect(M.S.cards.find((card) => card.id === "api#43")?.state).toBe("needs");
  });

  it("retries the merge, toasts it, and moves the card into the queue", async () => {
    seed("mobile", { ...stopped, currentCardId: idOf("mobile#210") });
    show("mobile");
    await screen.findByText("Waiting for you");
    press("Retry", region("Needs you"));
    await vi.waitFor(() => expect(toasts()).toContain("Retrying the merge of mobile#210"));
    expect(daemon.routes()).toContain(`POST /v1/cards/${idOf("mobile#210")}/merge/retry`);
    await vi.waitFor(() => expect(screen.queryByRole("region", { name: "Needs you" })).toBeNull());
    const waiting = region("Waiting to merge");
    expect(within(waiting).getByRole("article", { name: /^mobile#210 / })).toBeInTheDocument();
  });

  it("shows the daemon's own sentence when it refuses, and keeps the Retry", async () => {
    const sentence = "That merge has moved on, so there is nothing to retry.";
    daemon.refuseNext(`POST /v1/cards/${idOf("mobile#210")}/merge/retry`, 422, "refused", sentence);
    show("mobile");
    await screen.findByText("Idle");
    press("Retry", region("Needs you"));
    await vi.waitFor(() => expect(toasts()).toEqual([sentence]));
    expect(within(region("Needs you")).getByRole("button", { name: "Retry" })).toBeEnabled();
  });
});

describe("Undo", () => {
  it("puts a delivered merge back, toasts it, and takes it off the list", async () => {
    seed("api", {
      history: [historyItem({ cardId: idOf("api#41"), key: "api#41", title: "x", canUndo: true })],
    });
    show("api");
    const delivered = await screen.findByRole("region", { name: "Delivered" });
    press("Undo", delivered);
    await vi.waitFor(() => expect(toasts()).toContain("Undid the merge of api#41"));
    expect(daemon.routes()).toContain(`POST /v1/cards/${idOf("api#41")}/merge/undo`);
    await vi.waitFor(() => expect(screen.queryByRole("region", { name: "Delivered" })).toBeNull());
  });

  it("shows the daemon's own sentence when it refuses", async () => {
    const sentence = "Other cards were merged after this one, so it cannot be undone.";
    seed("api", {
      history: [historyItem({ cardId: idOf("api#41"), key: "api#41", title: "x", canUndo: true })],
    });
    daemon.refuseNext(`POST /v1/cards/${idOf("api#41")}/merge/undo`, 422, "refused", sentence);
    show("api");
    press("Undo", await screen.findByRole("region", { name: "Delivered" }));
    await vi.waitFor(() => expect(toasts()).toEqual([sentence]));
    expect(screen.getByRole("button", { name: "Undo" })).toBeEnabled();
  });
});

describe("the Integrator chat", () => {
  it("switches the project to its chat view", async () => {
    show("api");
    await heading();
    press("Open Integrator chat");
    expect(M.S.route).toMatchObject({ page: "project", pid: "api", view: "chat" });
  });
});
