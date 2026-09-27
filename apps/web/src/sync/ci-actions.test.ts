import type { Card as WireCard } from "@marshal/protocol";
import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import type { Marshal } from "~/mock";
import type { Ctx } from "~/mock/context";
import type { Card } from "~/mock/types";
import { cardId, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { ciOnDaemon, simulateOnDaemon, simulateReal, simulateSynthetic } from "./ci-actions";

/*
 * The two Simulate CI failure modes (N28, B6.4). The daemon owns both: the synthetic mode injects a
 * failed run through the CI monitor's own path, and the real mode pushes a deliberately failing
 * change to the card's own branch after a confirmation. The card's badge and the project's CI health
 * follow from the daemon's own events, so these tests assert the request that was made, the words
 * the person reads, and that nothing is asked before the question is answered.
 */

/** The sections with the cards on the daemon, which is what the cutover of S5a says. */
const CARDS_ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const };

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

const api41 = (fields: Partial<WireCard> = {}): WireCard =>
  wireCard({
    projectId: "api",
    number: 41,
    title: "Fix token refresh on login",
    state: "working",
    branch: "marshal/41-fix-token-refresh",
    ...fields,
  });

/** The address of a card's route as the client sends it: the daemon's own id, escaped in the path. */
const route = (method: string, key: string, action = ""): string => {
  const number = Number(key.split("#")[1] ?? "0");
  return `${method} /v1/cards/${encodeURIComponent(cardId(number))}${action ? `/${action}` : ""}`;
};

interface Fixture {
  M: Marshal;
  ctx: Ctx;
  d: FakeDaemon;
}

/**
 * A store that follows a fake daemon. Its health answer is the golden one, whose mode is `dev`, so
 * the daemon's own routes are the ones the store sees; a test that means a normal daemon changes the
 * answer itself.
 */
async function setup(cards: readonly WireCard[] = [api41()]): Promise<Fixture> {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards });
  daemon = d;
  const M = createTestMarshal({ data: d.data, sections: CARDS_ON_DAEMON });
  await d.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  await settled(d);
  return { M, ctx: contextOf(M), d };
}

/** Waits until the app has stopped asking for boards, so nothing lands on top of a change a test made. */
async function settled(d: FakeDaemon): Promise<void> {
  let last = -1;
  await vi.waitFor(() => {
    const loads = d.routes().filter((one) => one.endsWith("/board")).length;
    const steady = loads > 0 && loads === last;
    last = loads;
    expect(steady).toBe(true);
  });
}

function storeCard(M: Marshal, id: string): Card {
  const card = M.card(id);
  if (!card) throw new Error(`the store has no card ${id}`);
  return card;
}

/** The last sentence the person read. */
const said = (M: Marshal): string => M.S.toasts.at(-1)?.msg ?? "";

/** Answers the confirmation that is up, which is what the dialog's own button does. */
function answerDialog(ctx: Ctx): void {
  const dialog = ctx.S.dialog;
  if (!dialog) throw new Error("no confirmation is up");
  dialog.run();
}

describe("simulateOnDaemon", () => {
  it("is true for a card the daemon made while that daemon runs in dev mode", async () => {
    const { M, ctx } = await setup();
    expect(ctx.env.data?.connection.mode()).toBe("dev");
    expect(simulateOnDaemon(ctx, storeCard(M, "api#41"))).toBe(true);
  });

  it("is false for a normal daemon and for a card the daemon does not know", async () => {
    const { M, ctx } = await setup();
    vi.spyOn(ctx.env.data?.connection as { mode: () => string }, "mode").mockReturnValue("normal");
    expect(simulateOnDaemon(ctx, storeCard(M, "api#41"))).toBe(false);
    const mine = storeCard(M, "api#41");
    delete mine.daemonId;
    expect(simulateOnDaemon(ctx, mine)).toBe(false);
  });
});

describe("ciOnDaemon", () => {
  it("is true for a card a daemon sent, whatever its mode", async () => {
    const { M, ctx } = await setup();
    vi.spyOn(ctx.env.data?.connection as { mode: () => string }, "mode").mockReturnValue("normal");
    expect(ciOnDaemon(ctx, storeCard(M, "api#41"))).toBe(true);
  });

  it("is false with no daemon, even though the mirror gave the card its id", () => {
    // The store the card menu is drawn in has no daemon at all, and the mirror still fills a
    // `daemonId` for every card it applies, so the ownership question cannot be `daemonId` alone.
    const M = createTestMarshal();
    const ctx = contextOf(M);
    expect(ctx.env.data).toBeNull();
    expect(storeCard(M, "api#41").daemonId).toBeTruthy();
    expect(ciOnDaemon(ctx, storeCard(M, "api#41"))).toBe(false);
  });
});

describe("simulateSynthetic", () => {
  it("asks the daemon for a synthetic run and says what it did", async () => {
    const { M, ctx, d } = await setup();
    expect(await simulateSynthetic(ctx, "api#41")).toBe(true);
    expect(d.bodies(route("POST", "api#41", "ci-failure"))).toEqual([{ mode: "synthetic" }]);
    expect(said(M)).toBe("Simulated a CI failure on api-gateway #41");
  });

  it("asks for nothing for a card the daemon does not know, and says why", async () => {
    const { M, ctx, d } = await setup();
    const mine = storeCard(M, "api#41");
    delete mine.daemonId;
    expect(await simulateSynthetic(ctx, "api#41")).toBe(false);
    expect(d.bodies(route("POST", "api#41", "ci-failure"))).toEqual([]);
    expect(said(M)).toBe("Marshal is not connected to its daemon.");
  });

  it("shows the daemon's own sentence when it refuses", async () => {
    const { M, ctx, d } = await setup();
    d.refuseNext(route("POST", "api#41", "ci-failure"), 422, "refused", "That card has no branch.");
    expect(await simulateSynthetic(ctx, "api#41")).toBe(false);
    expect(said(M)).toBe("That card has no branch.");
  });
});

describe("simulateReal", () => {
  it("asks before it pushes, and names the branch in the question", async () => {
    const { ctx, d } = await setup();
    expect(simulateReal(ctx, "api#41")).toBe(true);
    expect(d.bodies(route("POST", "api#41", "ci-failure"))).toEqual([]);
    expect(ctx.S.dialog).toMatchObject({ title: "Simulate a real CI failure", destructive: true });
    expect(ctx.S.dialog?.message).toContain("marshal/41-fix-token-refresh");
    expect(ctx.S.dialog?.message).toContain("uses your Actions minutes");
  });

  it("pushes the real mode once the question is answered, and says where", async () => {
    const { M, ctx, d } = await setup();
    simulateReal(ctx, "api#41");
    answerDialog(ctx);
    // The request is recorded before its answer is read, so the sentence is waited for: it is what
    // says the push finished.
    await vi.waitFor(() =>
      expect(said(M)).toBe("Pushed a failing change to marshal/41-fix-token-refresh"),
    );
    expect(d.bodies(route("POST", "api#41", "ci-failure"))).toEqual([{ mode: "real" }]);
  });

  it("asks for nothing without a card the daemon knows", async () => {
    const { ctx, d } = await setup();
    expect(simulateReal(ctx, "api#9999")).toBe(false);
    expect(ctx.S.dialog).toBeNull();
    expect(d.bodies(route("POST", "api#9999", "ci-failure"))).toEqual([]);
  });
});

describe("the store's own action", () => {
  it("sends the synthetic mode to the daemon for a card the daemon made", async () => {
    const { M, d } = await setup();
    M.simulateCiFailure("api#41");
    await vi.waitFor(() => expect(said(M)).toBe("Simulated a CI failure on api-gateway #41"));
    expect(d.bodies(route("POST", "api#41", "ci-failure"))).toEqual([{ mode: "synthetic" }]);
  });

  it("sends the real mode to the daemon once the question is answered", async () => {
    const { M, ctx, d } = await setup();
    M.simulateCiFailureReal("api#41");
    answerDialog(ctx);
    await vi.waitFor(() =>
      expect(said(M)).toBe("Pushed a failing change to marshal/41-fix-token-refresh"),
    );
    expect(d.bodies(route("POST", "api#41", "ci-failure"))).toEqual([{ mode: "real" }]);
  });

  it("asks for nothing when the daemon is not in dev mode", async () => {
    const { M, ctx, d } = await setup();
    vi.spyOn(ctx.env.data?.connection as { mode: () => string }, "mode").mockReturnValue("normal");
    M.simulateCiFailureReal("api#41");
    expect(ctx.S.dialog).toBeNull();
    expect(d.bodies(route("POST", "api#41", "ci-failure"))).toEqual([]);
  });
});
