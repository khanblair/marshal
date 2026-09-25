import type { Health, WhoAmI } from "@marshal/protocol";
import { expect, vi } from "vitest";
import { wireError } from "../api-error";
import { createConnection } from "../connection";
import { type PageTargets, watchPage } from "../page-signals";
import { createTokenStore } from "../token";
import { golden } from "./golden";
import { memoryStorage } from "./memory-storage";

export const HEALTH = golden<Health>("health");
const WHO = golden<WhoAmI>("whoami");
const UNAUTHORIZED_STATUS = 401;
export const UNAUTHORIZED = () =>
  wireError(UNAUTHORIZED_STATUS, {
    code: "unauthorized",
    message: "Sign in again to use Marshal.",
  });

export const flush = () => vi.advanceTimersByTimeAsync(0);

export function setup() {
  const calls: string[] = [];
  const api = {
    health: vi.fn(async (_options?: { signal?: AbortSignal; timeoutMs?: number }) => {
      calls.push("health");
      return HEALTH;
    }),
    whoami: vi.fn(async (_options?: { signal?: AbortSignal; timeoutMs?: number }) => {
      calls.push("whoami");
      return WHO;
    }),
  };
  const storage = memoryStorage();
  const tokens = createTokenStore({ storage });
  const load = vi.spyOn(tokens, "load").mockImplementation(async () => {
    calls.push("load");
    return tokens.get();
  });
  const stream = { start: vi.fn(), stop: vi.fn() };
  const page = {
    window: new EventTarget(),
    document: Object.assign(new EventTarget(), { visibilityState: "visible" }),
  };
  const connection = createConnection({
    api,
    tokens,
    stream,
    watchPage: watchPage(page as PageTargets),
  });
  const reconnected = vi.fn();
  connection.onReconnected(reconnected);
  return { api, tokens, load, stream, page, connection, reconnected, calls };
}

export type Setup = ReturnType<typeof setup>;

/** Starts and lets the first check finish. */
export async function online(t: Setup) {
  t.connection.start();
  await flush();
  expect(t.connection.state()).toBe("online");
}

/** A drop of the stream while online, then its return. */
export async function dropAndReturn(t: Setup) {
  t.connection.streamState("waiting", { attempts: 1, error: null });
  expect(t.connection.state()).toBe("reconnecting");
  t.connection.streamState("open", { attempts: 1, error: null });
}
