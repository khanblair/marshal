import type { Card as WireCard } from "@marshal/protocol";
import { afterEach, describe, expect, it } from "vitest";
import type { SleepNotice } from "~/data/mappers/notices";
import type { Marshal } from "~/mock";
import type { Ctx } from "~/mock/context";
import { wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createSyncedMarshal, MOCK_HISTORY } from "~/testing/test-store";

// Section S23: the four calls a person makes on a sleep notice once the daemon holds the notices.
// Each asks the daemon and lets its own `notice.dismissed` bring the list the screens then show,
// so a call never draws its change on the client alone. The fake daemon holds the golden
// `notice-list` - one sleep group naming two cards - and the two cards it names, because a notice
// whose cards the store cannot place is not drawn at all.

const C3 = "01JD7Q4M2X8K9V0P5T3RB6NHC3";
const C4 = "01JD7Q4M2X8K9V0P5T3RB6NHC4";
const NOTICE = "sleep:web-dashboard";
/** The client escapes a notice id in its address, which is the key the fake daemon's calls are under. */
const WIRE = encodeURIComponent(NOTICE);
const ACTION = `POST /v1/notices/${WIRE}/actions`;

/** The store's sections: the notices are the daemon's, the hold controls stay the mock's. */
const SECTIONS = { ...MOCK_HISTORY, S23: "daemon" as const };

/** The two cards the golden notice names, held by the daemon under those exact ids. */
const CARDS: readonly WireCard[] = [C3, C4].map((id, i) =>
  wireCard({
    id,
    projectId: "web",
    number: i + 1,
    title: i === 0 ? "Refresh the token" : "CSV export",
    state: "ready",
    session: "awake",
  }),
);

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

async function setup(): Promise<{ M: Marshal; ctx: Ctx; d: FakeDaemon }> {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards: CARDS });
  daemon = d;
  const M = await createSyncedMarshal(d, { sections: SECTIONS });
  // The load's own calls are not part of what a test asserts.
  d.calls.length = 0;
  return { M, ctx: contextOf(M), d };
}

const sleepNotice = (M: Marshal): SleepNotice | undefined =>
  M.S.notices.find((one): one is SleepNotice => one.kind === "sleep");

const toasts = (M: Marshal): string[] => M.S.toasts.map((toast) => toast.msg);

describe("the notice calls on the daemon", () => {
  it("keeps one card awake: the call, the toast, and the group the daemon sends back", async () => {
    const { M, d } = await setup();
    expect(sleepNotice(M)?.cards).toEqual(["web#1", "web#2"]);
    expect(await M.keepAwake("web#1")).toBe(true);
    expect(d.bodies(ACTION)).toEqual([{ action: "keep-awake", cardId: C3 }]);
    expect(toasts(M)).toEqual(["Kept awake for 15 more minutes"]);
    expect(sleepNotice(M)?.cards).toEqual(["web#2"]);
  });

  it("keeps every card a notice names awake, and drops the notice", async () => {
    const { M, d } = await setup();
    expect(await M.keepAllAwake()).toBe(true);
    expect(d.bodies(ACTION)).toEqual([{ action: "keep-all" }]);
    expect(toasts(M)).toEqual(["Kept 2 cards awake"]);
    expect(sleepNotice(M)).toBeUndefined();
  });

  it("sleeps every card a notice names now, and says how many went", async () => {
    const { M, d } = await setup();
    expect(await M.sleepAll()).toBe(true);
    expect(d.bodies(ACTION)).toEqual([{ action: "sleep-all" }]);
    expect(toasts(M)).toEqual(["2 cards asleep"]);
    expect(sleepNotice(M)).toBeUndefined();
  });

  it("asks the daemon for nothing when no notice names the card", async () => {
    const { M, d } = await setup();
    expect(await M.keepAwake("api#39")).toBe(false);
    expect(d.routes()).toEqual([]);
    expect(toasts(M)).toEqual([]);
    expect(sleepNotice(M)?.cards).toEqual(["web#1", "web#2"]);
  });

  it("shows the daemon's own sentence and keeps the notice when a call is refused", async () => {
    const { M, d } = await setup();
    d.refuseNext(ACTION, 422, "refused", "Working cards don't sleep. Pause the card first.");
    expect(await M.sleepAll()).toBe(false);
    expect(toasts(M)).toEqual(["Working cards don't sleep. Pause the card first."]);
    expect(sleepNotice(M)?.cards).toEqual(["web#1", "web#2"]);
  });

  it("takes one notice off the panel without touching a card", async () => {
    const { M, d } = await setup();
    expect(await M.dismissNotice(NOTICE)).toBe(true);
    expect(d.routes()).toContain(`DELETE /v1/notices/${WIRE}`);
    expect(sleepNotice(M)).toBeUndefined();
  });
});
