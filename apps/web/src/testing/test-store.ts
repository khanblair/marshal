/**
 * The store for unit tests. It is built the way the app builds it, then filled through the same
 * functions the daemon's snapshots go through (`applyProjectSnapshot`, `applyAgentCatalog`), so
 * tests run the real mirror instead of a private seed.
 */

import type { AgentCatalog } from "@marshal/protocol";
import { expect, vi } from "vitest";
import { toDaemonProject } from "~/data/mappers/project";
import { isDaemon, type SectionId, type SectionStatus, sectionStatus } from "~/data/sections";
import { type Ctx, createContext, type Env, sectionsOf } from "~/mock/context";
import { createMarshalIn, type Marshal } from "~/mock/marshal";
import { overlayPrototypeCi } from "~/mock/testing/prototype-ci";
import { applyAgentCatalog } from "~/sync/agents";
import { applyProjectSnapshot } from "~/sync/projects";
import { PROTOTYPE_CATALOG } from "./agents";
import type { FakeDaemon } from "./fake-daemon";
import { PROTOTYPE_PROJECTS } from "./projects";
import { applyPrototypeCards } from "./prototype-cards";

const contexts = new WeakMap<Marshal, Ctx>();

/**
 * The sections with the cards still on the mock: what a test that exercises the mock's own path
 * asks for. It says so out loud rather than relying on the register, so the test keeps testing the
 * mock on the day S5a switches.
 */
export const MOCK_CARDS: Readonly<Record<SectionId, SectionStatus>> = {
  ...sectionStatus,
  S5a: "mock",
};

/** The sections with the cards on the daemon, which is what the cutover of S5a says. */
export const DAEMON_CARDS: Readonly<Record<SectionId, SectionStatus>> = {
  ...sectionStatus,
  S5a: "daemon",
};

/**
 * The sections with a card's chat and activity still on the mock, for a test that exercises the
 * mock's own history. S8a and S10 are switched together in practice, and a test that means the mock
 * says so rather than relying on the register. The rest of the mock's conversations are pinned with
 * them: the project chats (S17), whose mock chats the reservoir stops supplying once the section is
 * the daemon's, and the hold controls (S7c), which with no daemon could only say "not connected".
 */
export const MOCK_HISTORY: Readonly<Record<SectionId, SectionStatus>> = {
  ...sectionStatus,
  S7c: "mock",
  S8a: "mock",
  S10: "mock",
  S17: "mock",
};

/** The sections with a card's chat and activity on the daemon, which is what their cutover says. */
export const DAEMON_HISTORY: Readonly<Record<SectionId, SectionStatus>> = {
  ...sectionStatus,
  S8a: "daemon",
  S10: "daemon",
};

/**
 * The sections with the mock's cards and a card's chat and activity all still on the mock, for a
 * test whose subject is the mock's own store: its seed, or the objects its timers keep. It is
 * `MOCK_CARDS` and `MOCK_HISTORY` together, and it has to be both: pinning only the history puts the
 * cards on the daemon, and the test then compares against a store the mock never built.
 */
export const MOCK_CARDS_AND_HISTORY: Readonly<Record<SectionId, SectionStatus>> = {
  ...MOCK_CARDS,
  S7c: "mock",
  S8a: "mock",
  S10: "mock",
  S17: "mock",
};

/**
 * The sections of the person (the profile S2a, the saved views S6a, the screen preferences S32, and
 * the onboarding progress S31a) on the mock, for a test that exercises the mock's own person: the
 * prototype's people, its seeded saved views, and a profile it saves without a daemon. It says so out
 * loud rather than relying on the register, so the test keeps testing the mock on the day these
 * sections switch.
 */
export const MOCK_PERSON_SECTIONS = {
  S2a: "mock",
  S6a: "mock",
  S32: "mock",
  S31a: "mock",
} as const satisfies Partial<Record<SectionId, SectionStatus>>;

/** The register with only the person's sections on the mock. To add them to another table, spread `MOCK_PERSON_SECTIONS`. */
export const MOCK_PERSON: Readonly<Record<SectionId, SectionStatus>> = {
  ...sectionStatus,
  ...MOCK_PERSON_SECTIONS,
};

/** The same sections on the daemon, which is what their cutover says. */
export const DAEMON_PERSON: Readonly<Record<SectionId, SectionStatus>> = {
  ...sectionStatus,
  S2a: "daemon",
  S6a: "daemon",
  S32: "daemon",
  S31a: "daemon",
};

/** The context behind a store made here, for a test that fills the store through the mirror. */
export function contextOf(M: Marshal): Ctx {
  const ctx = contexts.get(M);
  if (!ctx) throw new Error("this store was not made by createTestMarshal");
  return ctx;
}

/** The environment of a test store: no daemon, no storage, and the window's own size and hash. */
export function testEnv(overrides: Partial<Env> = {}): Env {
  return {
    hash: window.location.hash,
    storage: null,
    viewport: { w: window.innerWidth, h: window.innerHeight },
    applyTheme: () => {},
    data: null,
    ...overrides,
  };
}

/** Fills a store with the prototype's three projects, its three agents, and its 29 cards, as if the daemon had sent them. */
function applyPrototypeData(ctx: Ctx): void {
  applyProjectSnapshot(ctx, PROTOTYPE_PROJECTS.map(toDaemonProject));
  overlayPrototypeCi(ctx.S);
  applyAgentCatalog(ctx, PROTOTYPE_CATALOG);
  // Once S5a is the daemon's, the reservoir keeps the mock's own cards out of the store for good, so
  // the tests that draw a board get them the way the app gets them: through the real mirror.
  if (isDaemon("S5a", sectionsOf(ctx.env))) applyPrototypeCards(ctx);
}

/**
 * A context with the prototype's projects and agents, unless the test gives it a daemon: then the daemon
 * says what exists, and `startSync` fills the store from it.
 */
export function createTestContext(overrides: Partial<Env> = {}): Ctx {
  const ctx = createContext(testEnv(overrides));
  if (!ctx.env.data) applyPrototypeData(ctx);
  return ctx;
}

/** A whole store like the app's, for a test that needs its own instead of the shared `M`. */
export function createTestMarshal(overrides: Partial<Env> = {}): Marshal {
  const ctx = createTestContext(overrides);
  const M = createMarshalIn(ctx);
  contexts.set(M, ctx);
  return M;
}

/** A store that follows a fake daemon, waited for until it has the daemon's first snapshots. */
export async function createSyncedMarshal(
  daemon: FakeDaemon,
  overrides: Partial<Env> = {},
): Promise<Marshal> {
  const M = createTestMarshal({ data: daemon.data, ...overrides });
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  return M;
}

/**
 * Puts a catalog in a store through the mirror, for one test, and returns the function that puts
 * the prototype's agents back, so the next test starts as every test does.
 */
export function useCatalog(M: Marshal, catalog: AgentCatalog): () => void {
  applyAgentCatalog(contextOf(M), catalog);
  return () => applyAgentCatalog(contextOf(M), PROTOTYPE_CATALOG);
}
