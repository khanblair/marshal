/**
 * The store for unit tests. It is built the way the app builds it, then filled through the same
 * functions the daemon's snapshots go through (`applyProjectSnapshot`, `applyAgentCatalog`), so
 * tests run the real mirror instead of a private seed.
 */

import type { AgentCatalog } from "@marshal/protocol";
import { expect, vi } from "vitest";
import { toDaemonProject } from "~/data/mappers/project";
import { type Ctx, createContext, type Env } from "~/mock/context";
import { createMarshalIn, type Marshal } from "~/mock/marshal";
import { overlayPrototypeCi } from "~/mock/testing/prototype-ci";
import { applyAgentCatalog } from "~/sync/agents";
import { applyProjectSnapshot } from "~/sync/projects";
import { PROTOTYPE_CATALOG } from "./agents";
import type { FakeDaemon } from "./fake-daemon";
import { PROTOTYPE_PROJECTS } from "./projects";

const contexts = new WeakMap<Marshal, Ctx>();

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

/** Fills a store with the prototype's three projects and its three agents, as if the daemon had sent them. */
function applyPrototypeData(ctx: Ctx): void {
  applyProjectSnapshot(ctx, PROTOTYPE_PROJECTS.map(toDaemonProject));
  overlayPrototypeCi(ctx.S);
  applyAgentCatalog(ctx, PROTOTYPE_CATALOG);
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
export async function createSyncedMarshal(daemon: FakeDaemon): Promise<Marshal> {
  const M = createTestMarshal({ data: daemon.data });
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
