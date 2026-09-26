import {
  EventTypeSessionStateChanged,
  SessionStateAsleep,
  SessionStateAwake,
  SessionStateWaking,
  type Card as WireCard,
  type Event as WireEvent,
} from "@marshal/protocol";
import { createEffect, createRoot } from "solid-js";
import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import type { Marshal } from "~/mock";
import type { Ctx } from "~/mock/context";
import { cardId, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { pause, pin, sleep, stopSession, wake } from "./card-hold";
import { applyCardSessionEvent } from "./card-session";

// Section S7c: the session hold (docs/backend-checklist.md B2.15). The cards are the daemon's (S5a)
// and so are the hold controls.
const ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const, S7c: "daemon" as const };

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

/** A working card with a live session, which is what the daemon has for a card that was started. */
const api41 = (fields: Partial<WireCard> = {}): WireCard =>
  wireCard({
    projectId: "api",
    number: 41,
    title: "Fix token refresh",
    state: "working",
    session: "awake",
    ...fields,
  });

const route = (action: string): string =>
  `POST /v1/cards/${encodeURIComponent(cardId(41))}/${action}`;

interface Fixture {
  M: Marshal;
  ctx: Ctx;
  d: FakeDaemon;
}

async function setup(card: WireCard = api41()): Promise<Fixture> {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards: [card] });
  daemon = d;
  const M = createTestMarshal({ data: d.data, sections: ON_DAEMON });
  await d.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  await settled(d);
  return { M, ctx: contextOf(M), d };
}

/**
 * Waits until the app has stopped asking for boards. The stream's first Resync makes it load once
 * more after `ready`, and that snapshot landing on top of a change a test just made would put the
 * card back the way the daemon had it before the change.
 */
async function settled(d: FakeDaemon): Promise<void> {
  let last = -1;
  await vi.waitFor(() => {
    const loads = d.routes().filter((one) => one.endsWith("/board")).length;
    const steady = loads > 0 && loads === last;
    last = loads;
    expect(steady).toBe(true);
  });
}

const toasts = (M: Marshal): string[] => M.S.toasts.map((toast) => toast.msg);

const stateEvent = (state: string): WireEvent => ({
  seq: 1,
  topic: `card:${cardId(41)}`,
  type: EventTypeSessionStateChanged,
  at: "2026-09-26T12:00:00.000Z",
  data: { cardId: cardId(41), state },
});

describe("pause", () => {
  it("asks the daemon and marks the card paused from its answer", async () => {
    const { M, ctx, d } = await setup();
    expect(await pause(ctx, "api#41")).toBe(true);
    expect(d.routes()).toContain(route("pause"));
    expect(M.card("api#41")?.paused).toBe(true);
    expect(toasts(M)).toEqual(["Card paused"]);
  });

  it("shows the daemon's own sentence, and changes nothing, when the card is not working", async () => {
    const { M, ctx, d } = await setup(api41({ state: "review" }));
    expect(await pause(ctx, "api#41")).toBe(false);
    expect(d.routes()).toContain(route("pause"));
    expect(M.card("api#41")?.paused).toBeFalsy();
    expect(toasts(M)).toEqual(["Only working cards can be paused."]);
  });

  it("does nothing for a card the store does not have", async () => {
    const { ctx, d } = await setup();
    expect(await pause(ctx, "api#9999")).toBe(false);
    expect(d.routes().filter((one) => one.endsWith("/pause"))).toEqual([]);
  });

  it("says Marshal is not connected when there is no daemon", async () => {
    const M = createTestMarshal({ sections: ON_DAEMON });
    const ctx = contextOf(M);
    ctx.S.cards.push({ ...M.S.cards[0]!, id: "api#41", daemonId: cardId(41) });
    expect(await pause(ctx, "api#41")).toBe(false);
    expect(toasts(M)).toContain(NOT_CONNECTED);
  });
});

describe("sleep", () => {
  it("asks the daemon to put a paused working card to sleep, and draws it asleep from the card the daemon sends", async () => {
    const { M, ctx, d } = await setup(api41({ paused: true }));
    expect(await sleep(ctx, "api#41")).toBe(true);
    expect(d.routes()).toContain(route("sleep"));
    expect(toasts(M)).toEqual(["Card asleep"]);
    // The daemon answers with no body: the card is asleep because it said so, as card.updated on the
    // project's topic, with nobody having the card open.
    await vi.waitFor(() =>
      expect(M.card("api#41")).toMatchObject({
        session: "asleep",
        asleep: true,
        waking: false,
        paused: true,
      }),
    );
    expect(M.S.openId).toBeNull();
  });

  it("shows the daemon's sentence when the card has no awake session", async () => {
    for (const session of [null, "stopped", "asleep"] as const) {
      const { M, ctx } = await setup(api41({ state: "review", session }));
      expect(await sleep(ctx, "api#41")).toBe(false);
      expect(toasts(M)).toEqual(["This card has no awake session."]);
      expect(M.card("api#41")?.session).toBe(session);
      daemon?.data.stop();
      daemon = null;
    }
  });

  it("does not draw a card asleep that the daemon refused to put to sleep", async () => {
    const { M, ctx } = await setup();
    expect(await sleep(ctx, "api#41")).toBe(false);
    expect(M.card("api#41")).toMatchObject({ session: "awake", asleep: false });
  });

  it("shows the daemon's sentence when a working card is asked to sleep", async () => {
    const { M, ctx } = await setup();
    expect(await sleep(ctx, "api#41")).toBe(false);
    expect(toasts(M)).toEqual(["Working cards don't sleep. Pause the card first."]);
  });

  it("shows the daemon's sentence when the card is waiting on the person", async () => {
    const { M, ctx } = await setup(api41({ state: "needs" }));
    expect(await sleep(ctx, "api#41")).toBe(false);
    expect(toasts(M)).toEqual(["This card is waiting on you, so it stays awake."]);
  });
});

describe("wake", () => {
  it("asks the daemon to wake the card, which goes waking and then awake and working again", async () => {
    const { M, ctx, d } = await setup(api41({ state: "review", session: "asleep" }));
    const seen: string[] = [];
    createRoot(() => {
      createEffect(() => {
        const card = M.card("api#41");
        if (card) seen.push(`${card.session}:${card.asleep}:${card.waking}:${card.state}`);
      });
    });
    expect(M.card("api#41")).toMatchObject({ asleep: true, waking: false });
    expect(await wake(ctx, "api#41")).toBe(true);
    expect(d.routes()).toContain(route("wake"));
    expect(toasts(M)).toEqual(["Session resumed"]);
    await vi.waitFor(() =>
      expect(M.card("api#41")).toMatchObject({
        session: "awake",
        asleep: false,
        waking: false,
        state: "working",
      }),
    );
    // Asleep, then waking with the card still asleep, then awake: nothing in between, and no flicker.
    expect(seen).toEqual([
      "asleep:true:false:review",
      "waking:true:true:review",
      "waking:true:true:working",
      "awake:false:false:working",
    ]);
  });

  it("shows the daemon's sentence for a session that has stopped, and says nothing is asleep for a card with none", async () => {
    const stopped = await setup(api41({ session: "stopped" }));
    expect(await wake(stopped.ctx, "api#41")).toBe(false);
    expect(toasts(stopped.M)).toEqual(["This card's session has stopped and cannot be resumed."]);
    daemon?.data.stop();
    daemon = null;
    const none = await setup(api41({ state: "backlog", session: null }));
    expect(await wake(none.ctx, "api#41")).toBe(false);
    expect(toasts(none.M)).toEqual(["Marshal cannot find that session. It may have been removed."]);
  });

  it("does nothing for a card the store does not have", async () => {
    const { ctx, d } = await setup();
    expect(await wake(ctx, "api#9999")).toBe(false);
    expect(d.routes().filter((one) => one.endsWith("/wake"))).toEqual([]);
  });
});

describe("pin", () => {
  it("pins an unpinned card and unpins a pinned one, each from the daemon's answer", async () => {
    const { M, ctx, d } = await setup();
    expect(await pin(ctx, "api#41")).toBe(true);
    expect(d.routes()).toContain(route("pin"));
    expect(M.card("api#41")?.pinned).toBe(true);
    expect(toasts(M)).toContain("Card pinned. It won't sleep.");

    expect(await pin(ctx, "api#41")).toBe(true);
    expect(d.routes()).toContain(route("unpin"));
    expect(M.card("api#41")?.pinned).toBe(false);
    expect(toasts(M)).toContain("Card unpinned");
  });

  it("refuses a second change to the same card while the first is still being saved", async () => {
    const { M, ctx } = await setup();
    const first = pin(ctx, "api#41");
    const second = await pin(ctx, "api#41");
    expect(second).toBe(false);
    expect(await first).toBe(true);
    expect(toasts(M)).toContain("That change is still being saved. Wait a moment and try again.");
  });
});

describe("the session's own state, on the open card's topic", () => {
  it("draws asleep and waking on the card, and clears both when the session is awake again", async () => {
    const { M, ctx } = await setup();
    applyCardSessionEvent(ctx, stateEvent(SessionStateAsleep));
    expect(M.card("api#41")).toMatchObject({ session: "asleep", asleep: true, waking: false });
    applyCardSessionEvent(ctx, stateEvent(SessionStateWaking));
    expect(M.card("api#41")).toMatchObject({ session: "waking", asleep: true, waking: true });
    applyCardSessionEvent(ctx, stateEvent(SessionStateAwake));
    expect(M.card("api#41")).toMatchObject({ session: "awake", asleep: false, waking: false });
  });

  it("agrees with the card.updated that says the same thing, in either order, without a flicker", async () => {
    const { M, ctx, d } = await setup();
    const seen: string[] = [];
    createRoot(() => {
      createEffect(() => {
        const card = M.card("api#41");
        if (card) seen.push(`${card.session}:${card.asleep}:${card.waking}`);
      });
    });
    // The card's own topic first, then the project's card.updated.
    applyCardSessionEvent(ctx, stateEvent(SessionStateAsleep));
    d.emit("project:api", "card.updated", { card: api41({ session: "asleep" }) });
    // The project's first, then the card's own topic.
    d.emit("project:api", "card.updated", { card: api41({ session: "waking" }) });
    await vi.waitFor(() => expect(M.card("api#41")?.waking).toBe(true));
    applyCardSessionEvent(ctx, stateEvent(SessionStateWaking));
    applyCardSessionEvent(ctx, stateEvent(SessionStateAwake));
    d.emit("project:api", "card.updated", { card: api41({ session: "awake" }) });
    await vi.waitFor(() => expect(M.card("api#41")?.asleep).toBe(false));
    expect(seen).toEqual([
      "awake:false:false",
      "asleep:true:false",
      "waking:true:true",
      "awake:false:false",
    ]);
  });

  it("ignores a state for a card the store does not have, and a state the app does not know", async () => {
    const { M, ctx } = await setup();
    applyCardSessionEvent(ctx, {
      ...stateEvent(SessionStateAsleep),
      data: { cardId: "no-such-card", state: SessionStateAsleep },
    });
    expect(M.card("api#41")?.asleep).toBeFalsy();
    applyCardSessionEvent(ctx, {
      ...stateEvent(SessionStateAsleep),
      data: { cardId: cardId(41), state: "napping" },
    });
    expect(M.card("api#41")).toMatchObject({ session: "awake", asleep: false });
  });

  it("leaves a project chat's session events alone: they carry no card", async () => {
    const { M, ctx } = await setup();
    applyCardSessionEvent(ctx, {
      ...stateEvent(SessionStateAsleep),
      topic: "chat:01M3CHAT",
      data: { cardId: "", chatId: "01M3CHAT", state: SessionStateAsleep },
    });
    expect(M.card("api#41")).toMatchObject({ session: "awake", asleep: false });
  });
});

describe("stopping a session (the Agents view)", () => {
  it("holds a working card and then puts it to sleep, with one toast", async () => {
    const { M, ctx, d } = await setup();
    expect(await stopSession(ctx, "api#41")).toBe(true);
    expect(d.routes().filter((one) => /\/(pause|sleep)$/.test(one))).toEqual([
      route("pause"),
      route("sleep"),
    ]);
    expect(toasts(M)).toEqual(["Session stopped"]);
    await vi.waitFor(() => expect(M.card("api#41")).toMatchObject({ asleep: true, paused: true }));
  });

  it("only puts a card to sleep that is not working, as the mock does", async () => {
    const { M, ctx, d } = await setup(api41({ state: "review" }));
    expect(await stopSession(ctx, "api#41")).toBe(true);
    expect(d.routes().filter((one) => one.endsWith("/pause"))).toEqual([]);
    await vi.waitFor(() => expect(M.card("api#41")).toMatchObject({ asleep: true, paused: false }));
  });

  it("stops at the daemon's refusal and leaves the card as it was", async () => {
    const { M, ctx, d } = await setup(api41({ state: "needs" }));
    expect(await stopSession(ctx, "api#41")).toBe(false);
    expect(toasts(M)).toEqual(["This card is waiting on you, so it stays awake."]);
    expect(d.routes().filter((one) => one.endsWith("/pause"))).toEqual([]);
    expect(M.card("api#41")?.asleep).toBe(false);
  });

  it("does nothing for a card the store does not have", async () => {
    const { ctx, d } = await setup();
    expect(await stopSession(ctx, "api#9999")).toBe(false);
    expect(d.routes().filter((one) => /\/(pause|sleep)$/.test(one))).toEqual([]);
  });
});

describe("releasing a pause with Start", () => {
  it("says the card resumed, and the card is no longer paused", async () => {
    const { M, ctx } = await setup(api41({ paused: true }));
    const { start } = await import("./card-actions");
    expect(await start(ctx, "api#41")).toBe(true);
    expect(toasts(M)).toEqual(["Card resumed"]);
    expect(M.card("api#41")?.paused).toBe(false);
  });
});
