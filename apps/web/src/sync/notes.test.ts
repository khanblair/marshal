import { afterEach, describe, expect, it } from "vitest";
import { sectionStatus } from "~/data/sections";
import type { Marshal } from "~/mock";
import type { Ctx } from "~/mock/context";
import { cardId, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createSyncedMarshal } from "~/testing/test-store";
import { saveCardNote } from "./notes";

/*
 * A card's note once section S14 is the daemon's: the one write the Notes tab makes. Reading is
 * `card-session.ts`'s `readOpenCard`, covered there and in `card-session`-adjacent tests; this file
 * is the save's own path, the same way `checkpoint-actions.test.ts` covers a restore's.
 */

const NOTES_ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const, S14: "daemon" as const };
const KEY = "api#41";
const CARD = cardId(41);
const NOT_CONNECTED = "Marshal is not connected to its daemon.";
const LONG_AGO = "2000-01-01T00:00:00.000Z";

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

const noteRoute = (): string => `PUT /v1/cards/${encodeURIComponent(CARD)}/note`;

const api41 = () =>
  wireCard({
    projectId: "api",
    number: 41,
    title: "Fix token refresh on login",
    state: "working",
    updatedAt: LONG_AGO,
  });

interface Fixture {
  M: Marshal;
  ctx: Ctx;
  d: FakeDaemon;
}

async function setup(): Promise<Fixture> {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards: [api41()] });
  daemon = d;
  const M = await createSyncedMarshal(d, { sections: NOTES_ON_DAEMON });
  return { M, ctx: contextOf(M), d };
}

describe("saving a card's note", () => {
  it("calls the note route and keeps the body and the daemon's own path", async () => {
    const { ctx, d } = await setup();
    expect(await saveCardNote(ctx, KEY, "Mine now")).toBe(true);
    expect(d.routes()).toContain(noteRoute());
    expect(d.bodies(noteRoute())).toEqual([{ body: "Mine now" }]);
    expect(ctx.S.notes?.[KEY]).toBe("Mine now");
    expect(ctx.S.noteInfo?.[KEY]?.path).toBe("api/cards/41-fix-token-refresh-on-login.md");
    expect(ctx.S.noteInfo?.[KEY]?.author).toBe("person");
    expect(ctx.S.noteInfo?.[KEY]?.updatedAt).not.toBeNull();
  });

  it("shows the toast the way every other card write does", async () => {
    const { ctx, M } = await setup();
    await saveCardNote(ctx, KEY, "Mine now");
    expect(M.S.toasts.at(-1)?.msg).toBe("Note saved");
  });

  it("does nothing for a card with no daemon id", async () => {
    const { ctx, M, d } = await setup();
    const card = M.card(KEY);
    if (card) card.daemonId = undefined;
    expect(await saveCardNote(ctx, KEY, "Mine now")).toBe(false);
    expect(d.routes()).not.toContain(noteRoute());
  });

  it("warns, and calls nothing, when there is no daemon connection", async () => {
    const { ctx, M } = await setup();
    ctx.env.data = undefined;
    expect(await saveCardNote(ctx, KEY, "Mine now")).toBe(false);
    expect(M.S.toasts.at(-1)?.msg).toBe(NOT_CONNECTED);
  });
});
