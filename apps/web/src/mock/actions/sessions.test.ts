import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import { wireCard } from "~/testing/fake-cards";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal, MOCK_CARDS } from "~/testing/test-store";
import type { SleepNotice } from "../types";

// The idle-card notice belongs to the idle timer, which is Phase 5. While the hold controls are the
// daemon's, its three actions say they are not available and change nothing: they must never mark a
// daemon card asleep on the screen alone or promise the person something the daemon does not do.
// The only way a daemon card gets into one is the mock's seeded notice, which names the prototype's
// card keys: a daemon that holds the prototype fixture has cards under them.

const ON_DAEMON = { ...sectionStatus, S5a: "daemon" as const, S7c: "daemon" as const };
/** The mock's own cards and hold controls, whatever the register says. */
const MOCK_HOLD = { ...MOCK_CARDS, S7c: "mock" as const };

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

/** A store that follows a daemon holding the three cards the seeded sleep notice names. */
async function storeWithNoticeCards() {
  const cards = [
    wireCard({
      projectId: "api",
      number: 39,
      title: "Structured logging",
      state: "review",
      session: "awake",
    }),
    wireCard({
      projectId: "api",
      number: 36,
      title: "Remove v1 routes",
      state: "ready",
      session: "awake",
    }),
    wireCard({
      projectId: "web",
      number: 116,
      title: "CSV export",
      state: "ready",
      session: "awake",
    }),
  ];
  daemon = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards });
  const M = createTestMarshal({ data: daemon.data, sections: ON_DAEMON });
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  return { M, ctx: contextOf(M), d: daemon };
}

const sleepNotice = (M: { S: { notices: readonly unknown[] } }): SleepNotice | undefined =>
  M.S.notices.find((n): n is SleepNotice => (n as { kind?: string }).kind === "sleep");

const toasts = (M: { S: { toasts: readonly { msg: string }[] } }): string[] =>
  M.S.toasts.map((t) => t.msg);

describe("the sleep notice's actions while the hold controls are the daemon's", () => {
  it("shows the seeded notice, which names the daemon's cards by their keys", async () => {
    const { M } = await storeWithNoticeCards();
    expect(sleepNotice(M)?.cards).toEqual(["api#39", "api#36", "web#116"]);
    expect(M.card("api#39")?.daemonId).toBeDefined();
  });

  it("says Keep awake is not available, and points at Pin, without touching the card or the notice", async () => {
    const { M, d } = await storeWithNoticeCards();
    M.keepAwake("api#39");
    expect(toasts(M)).toEqual([
      "Keeping cards awake for a while is not available yet. Pin a card to keep it awake.",
    ]);
    expect(sleepNotice(M)?.cards).toContain("api#39");
    expect(M.card("api#39")).toMatchObject({ asleep: false, pinned: false });
    expect(d.routes().filter((one) => one.startsWith("POST"))).toEqual([]);
  });

  it("says Keep all awake is not available, and leaves the notice where it is", async () => {
    const { M } = await storeWithNoticeCards();
    M.keepAllAwake();
    expect(toasts(M)).toEqual([
      "Keeping cards awake for a while is not available yet. Pin a card to keep it awake.",
    ]);
    expect(sleepNotice(M)?.cards).toHaveLength(3);
  });

  it("does not put a card to sleep with Sleep all, and says so, and asks the daemon for nothing", async () => {
    const { M, d } = await storeWithNoticeCards();
    M.sleepAll();
    expect(toasts(M)).toEqual([
      "Putting all these cards to sleep at once is not available yet. Use Sleep on each card.",
    ]);
    for (const key of ["api#39", "api#36", "web#116"]) {
      expect(M.card(key)).toMatchObject({ asleep: false, waking: false, session: "awake" });
    }
    expect(sleepNotice(M)?.cards).toHaveLength(3);
    expect(d.routes().filter((one) => one.startsWith("POST"))).toEqual([]);
  });
});

describe("the sleep notice's actions on the mock's own cards and hold controls", () => {
  it("keeps doing what the prototype did", () => {
    const M = createTestMarshal({ sections: MOCK_HOLD });
    expect(M.S.notices.some((n) => n.kind === "sleep")).toBe(true);
    const first = sleepNotice(M)?.cards[0] ?? "";
    M.keepAwake(first);
    expect(toasts(M).at(-1)).toBe("Kept awake for 15 more minutes");
    expect(sleepNotice(M)?.cards).not.toContain(first);
    const rest = sleepNotice(M)?.cards ?? [];
    M.sleepAll();
    expect(toasts(M).at(-1)).toBe(`${rest.length} cards asleep`);
    for (const key of rest) expect(M.card(key)?.asleep).toBe(true);
    expect(sleepNotice(M)).toBeUndefined();
  });

  it("keeps all awake, and drops the notice", () => {
    const M = createTestMarshal({ sections: MOCK_HOLD });
    const count = sleepNotice(M)?.cards.length ?? 0;
    M.keepAllAwake();
    expect(toasts(M).at(-1)).toBe(`Kept ${count} cards awake`);
    expect(sleepNotice(M)).toBeUndefined();
  });
});
