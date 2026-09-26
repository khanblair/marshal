/**
 * The store a component test follows when section S5a is on the daemon.
 *
 * The screens read one store, `M` from `~/mock`, which is built the first time that module is read
 * (`window.M ?? boot()`, and the test setup puts a lazy `window.M` in place). So a test file that
 * imports this module *before* anything that reads the store gives that store a fake daemon to
 * follow: the daemon holds the prototype's projects, its agents, and its 29 cards, and the store is
 * put on `window.M` before `~/mock` can build one of its own. The store is then the app's own, and
 * `M.quickAdd` and the rest of the card writes take the daemon's path, exactly as they will the day
 * the section switches.
 *
 * The hook below waits until the store has the daemon's first boards and has stopped asking for
 * them, once per file, before any test runs.
 */
import { afterAll, beforeAll, expect, vi } from "vitest";
import { applyCardSnapshot } from "~/sync/cards";
import { PROTOTYPE_CATALOG } from "./agents";
import { createFakeDaemon, type FakeDaemon } from "./fake-daemon";
import { PROTOTYPE_PROJECTS } from "./projects";
import { prototypeBoards, prototypeWireCards } from "./prototype-cards";
import { contextOf, createTestMarshal, DAEMON_CARDS } from "./test-store";

/** The daemon the component tests of this file drive. */
export const daemon: FakeDaemon = createFakeDaemon({
  projects: PROTOTYPE_PROJECTS,
  catalog: PROTOTYPE_CATALOG,
  cards: prototypeWireCards(),
});

/** The store the screens read, following `daemon`. */
// The hold controls (S7c) are pinned to the daemon too, so a test of the card panel, Home, or the
// Agents view exercises the path the app takes once the cards are the daemon's.
const M = createTestMarshal({
  data: daemon.data,
  sections: { ...DAEMON_CARDS, S7c: "daemon" },
});

// Before `~/mock` is read, so its own `window.M ?? boot()` finds this store and every component
// that reads `M` reads this one.
window.M = M;

/** Puts the daemon's 29 cards back, so a test that changed them does not change the next one. */
export function resetDaemonCards(): void {
  daemon.cards.splice(0, daemon.cards.length, ...prototypeWireCards());
}

/**
 * Puts them back in the store too, through the real mirror, for a test that created or removed a
 * card. The mirror is what gives a card the daemon's own id, which its routes take - a store
 * reseeded from the store-shaped fixture alone would hold cards no route can name.
 */
export function resetStoreCards(): void {
  applyCardSnapshot(contextOf(M), prototypeBoards());
}

/**
 * Waits until the app has stopped asking for boards. The stream's first Resync makes it load once
 * more after `ready`, and that second snapshot landing on top of a change a test just made would put
 * the card back the way the daemon had it before the change.
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

beforeAll(async () => {
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  await settled(daemon);
});

afterAll(() => daemon.data.stop());
