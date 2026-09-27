import type { Checkpoint } from "@marshal/protocol";
import { afterEach, describe, expect, it } from "vitest";
import type { ApiClient } from "~/data/api-client";
import { sectionStatus } from "~/data/sections";
import type { Marshal } from "~/mock";
import type { Ctx } from "~/mock/context";
import { cardId, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createSyncedMarshal, createTestMarshal } from "~/testing/test-store";
import { checkpointsOnDaemon, readCardCheckpoints, restoreCheckpoint } from "./checkpoint-actions";

/*
 * A card's restore points once the card activity is the daemon's (section S10, B5.3): the list the
 * activity tab draws, and the one write a person makes with it. A restore point is Marshal's own
 * commit, so the daemon owns both the list and the restore; the store mirrors what each route
 * answers, and the daemon's own sentence is what a refusal shows.
 */

/** The sections with the cards and the card activity on the daemon, which is what the cutover says. */
const CHECKPOINTS_ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const, S10: "daemon" as const };

const KEY = "api#41";
const CARD = cardId(41);
const NOT_CONNECTED = "Marshal is not connected to its daemon.";
/** A card time far enough in the past that any real clock the fake daemon answers with is later. */
const LONG_AGO = "2000-01-01T00:00:00.000Z";

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

const list = (): string => `GET /v1/cards/${encodeURIComponent(CARD)}/checkpoints`;
const restore = (cpId: string): string =>
  `POST /v1/cards/${encodeURIComponent(CARD)}/checkpoints/${encodeURIComponent(cpId)}/restore`;

const api41 = () =>
  wireCard({
    projectId: "api",
    number: 41,
    title: "Fix token refresh on login",
    state: "working",
    updatedAt: LONG_AGO,
  });

const cp = (fields: Partial<Checkpoint> = {}): Checkpoint => ({
  id: "01JD7Q4M2X8K9V0P5T3RB6NHAE",
  cardId: CARD,
  sha: "9f2c1b7a4e6d85031234567890abcdef01234567",
  label: "before turn 3",
  createdAt: "2026-09-27T09:30:00.000Z",
  ...fields,
});

interface Fixture {
  M: Marshal;
  ctx: Ctx;
  d: FakeDaemon;
  api: ApiClient;
}

/** A store that follows a fake daemon holding one working card with two restore points. */
async function setup(
  checkpoints: readonly Checkpoint[] = [cp(), cp({ id: "cp2", label: "" })],
): Promise<Fixture> {
  const d = createFakeDaemon({
    projects: PROTOTYPE_PROJECTS,
    cards: [api41()],
    checkpoints: { [CARD]: checkpoints },
  });
  daemon = d;
  const M = await createSyncedMarshal(d, { sections: CHECKPOINTS_ON_DAEMON });
  const ctx = contextOf(M);
  const api = ctx.env.data?.api;
  if (!api) throw new Error("the synced store has no api client");
  return { M, ctx, d, api };
}

function storeCard(M: Marshal, id: string) {
  const card = M.card(id);
  if (!card) throw new Error(`the store has no card ${id}`);
  return card;
}

describe("reading a card's restore points", () => {
  it("is on section S10, and reads when the card activity is the daemon's", async () => {
    const { ctx } = await setup();
    expect(checkpointsOnDaemon(ctx)).toBe(true);
    // S10 is the daemon's for real, which is what the cutover says; the mock's own tab is a table
    // that still has it, and is what a section that had not switched would look like.
    const mockTabs = { ...sectionStatus, S10: "mock" as const };
    expect(checkpointsOnDaemon(contextOf(createTestMarshal({ sections: mockTabs })))).toBe(false);
  });

  it("puts the daemon's list in the store, newest first, with the time in ms", async () => {
    const { ctx, d, api } = await setup();
    await readCardCheckpoints(ctx, api, KEY, CARD);
    expect(d.routes()).toContain(list());
    expect(ctx.S.checkpoints[KEY]).toEqual([
      {
        id: cp().id,
        cardId: CARD,
        sha: cp().sha,
        label: "before turn 3",
        at: Date.parse(cp().createdAt),
      },
      { id: "cp2", cardId: CARD, sha: cp().sha, label: "", at: Date.parse(cp().createdAt) },
    ]);
  });

  it("leaves the store alone when the list arrives after the card has gone", async () => {
    const { ctx, api } = await setup();
    const kept = [{ id: "keep", cardId: CARD, sha: "a".repeat(40), label: "kept", at: 0 }];
    ctx.S.checkpoints[KEY] = kept;
    // The card is closed while the daemon is answering: the list is then for a row the store no
    // longer holds, and writing it would put it under whatever took that key next.
    ctx.S.cards = ctx.S.cards.filter((one) => one.id !== KEY);
    await readCardCheckpoints(ctx, api, KEY, CARD);
    expect(ctx.S.checkpoints[KEY]).toEqual(kept);
  });
});

describe("restoring a card to a restore point", () => {
  it("calls the restore route, draws the card it answers, and reads the list again", async () => {
    const { M, ctx, d } = await setup();
    expect(storeCard(M, KEY).upd).toBe(Date.parse(LONG_AGO));
    expect(await restoreCheckpoint(ctx, KEY, cp().id)).toBe(true);
    expect(d.routes()).toContain(restore(cp().id));
    // The restore route answers the card as it now is, so the store draws a card the daemon just
    // touched; and the list is read once more, because a card mid-turn when it was first read may
    // have gained a restore point since.
    expect(storeCard(M, KEY).upd).toBeGreaterThan(Date.parse(LONG_AGO));
    expect(d.routes().filter((one) => one === list())).toHaveLength(1);
  });

  it("sends no body, because the Restore button puts the worktree back alone", async () => {
    const { ctx, d } = await setup();
    expect(await restoreCheckpoint(ctx, KEY, cp().id)).toBe(true);
    expect(d.bodies(restore(cp().id))).toEqual([]);
  });

  it("shows the daemon's sentence and changes nothing when the restore is refused", async () => {
    const { M, ctx, d } = await setup();
    const sentence = "This card's agent is working. Wait for the turn to end, then restore.";
    d.refuseNext(restore(cp().id), 422, "refused", sentence);
    expect(await restoreCheckpoint(ctx, KEY, cp().id)).toBe(false);
    expect(M.S.toasts.at(-1)?.msg).toBe(sentence);
    expect(storeCard(M, KEY).upd).toBe(Date.parse(LONG_AGO));
  });

  it("does nothing for a card with no daemon id, which the mock made", async () => {
    const { M, ctx, d } = await setup();
    const card = storeCard(M, KEY);
    card.daemonId = undefined;
    expect(await restoreCheckpoint(ctx, KEY, cp().id)).toBe(false);
    expect(d.routes()).not.toContain(restore(cp().id));
  });

  it("warns, and calls nothing, when there is no daemon connection", async () => {
    const { M, ctx } = await setup();
    ctx.env.data = undefined;
    expect(await restoreCheckpoint(ctx, KEY, cp().id)).toBe(false);
    expect(M.S.toasts.at(-1)?.msg).toBe(NOT_CONNECTED);
  });
});
