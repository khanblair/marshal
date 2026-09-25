import { describe, expect, it, vi } from "vitest";
import { createDevTokenFetcher, DEV_TOKEN_PATH } from "./dev-token-client";
import { emptyAnswer, fakeFetch, jsonAnswer, textAnswer } from "./testing/fake-fetch";
import { brokenStorage, memoryStorage } from "./testing/memory-storage";
import { createTokenStore, TOKEN_KEY } from "./token";

const STORED = "stored-token-not-real";
const DEV = "dev-token-not-real";

describe("token store", () => {
  it("has no token at first", () => {
    expect(createTokenStore({ storage: memoryStorage() }).get()).toBeNull();
  });

  it("keeps a token in storage under marshal-token and clears it", () => {
    const storage = memoryStorage();
    const tokens = createTokenStore({ storage });
    tokens.set(STORED);
    expect(storage.values.get(TOKEN_KEY)).toBe(STORED);
    expect(TOKEN_KEY).toBe("marshal-token");
    expect(tokens.get()).toBe(STORED);
    tokens.clear();
    expect(storage.values.has(TOKEN_KEY)).toBe(false);
    expect(tokens.get()).toBeNull();
  });

  it("reads a token that another tab stored", () => {
    const storage = memoryStorage();
    const tokens = createTokenStore({ storage });
    storage.values.set(TOKEN_KEY, STORED);
    expect(tokens.get()).toBe(STORED);
  });

  it("tells listeners about each real change, and only until they stop", () => {
    const tokens = createTokenStore({ storage: memoryStorage() });
    const listener = vi.fn();
    const stop = tokens.subscribe(listener);
    tokens.set("a");
    tokens.set("a");
    tokens.set("b");
    tokens.clear();
    tokens.clear();
    expect(listener).toHaveBeenCalledTimes(3);
    stop();
    tokens.set("c");
    expect(listener).toHaveBeenCalledTimes(3);
  });

  it("works without storage and with storage that throws, keeping the token in memory", () => {
    for (const storage of [null, brokenStorage()]) {
      const tokens = createTokenStore({ storage });
      expect(tokens.get()).toBeNull();
      tokens.set(STORED);
      expect(tokens.get()).toBe(STORED);
      tokens.clear();
      expect(tokens.get()).toBeNull();
    }
  });
});

describe("token store load", () => {
  it("never asks for the dev token in a normal build", async () => {
    const fetchDevToken = vi.fn(async () => DEV);
    const tokens = createTokenStore({ storage: memoryStorage(), dev: false, fetchDevToken });
    expect(await tokens.load()).toBeNull();
    expect(fetchDevToken).not.toHaveBeenCalled();
  });

  it("uses a stored token as it is, without asking the dev server", async () => {
    const fetchDevToken = vi.fn(async () => DEV);
    const tokens = createTokenStore({
      storage: memoryStorage({ [TOKEN_KEY]: STORED }),
      dev: true,
      fetchDevToken,
    });
    expect(await tokens.load()).toBe(STORED);
    expect(fetchDevToken).not.toHaveBeenCalled();
  });

  it("asks the dev server in the dev build and keeps the answer in memory only", async () => {
    const storage = memoryStorage();
    const tokens = createTokenStore({ storage, dev: true, fetchDevToken: async () => DEV });
    const listener = vi.fn();
    tokens.subscribe(listener);
    expect(await tokens.load()).toBe(DEV);
    expect(tokens.get()).toBe(DEV);
    expect(storage.writes).toEqual([]);
    expect(storage.values.size).toBe(0);
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it("reports no token yet when the dev daemon has not started, without throwing", async () => {
    const tokens = createTokenStore({
      storage: memoryStorage(),
      dev: true,
      fetchDevToken: async () => null,
    });
    await expect(tokens.load()).resolves.toBeNull();
    expect(tokens.get()).toBeNull();
  });

  it("asks again each time, so a reset data folder is followed", async () => {
    const answers: (string | null)[] = [DEV, "second-dev-token", null];
    const tokens = createTokenStore({
      storage: memoryStorage(),
      dev: true,
      fetchDevToken: async () => answers.shift() ?? null,
    });
    const listener = vi.fn();
    tokens.subscribe(listener);
    expect(await tokens.load()).toBe(DEV);
    expect(await tokens.load()).toBe("second-dev-token");
    expect(await tokens.load()).toBeNull();
    expect(listener).toHaveBeenCalledTimes(3);
  });

  it("keeps what it holds when the dev server does not answer", async () => {
    let fail = false;
    const tokens = createTokenStore({
      storage: memoryStorage(),
      dev: true,
      fetchDevToken: async () => {
        if (fail) throw new TypeError("Failed to fetch");
        return DEV;
      },
    });
    await tokens.load();
    fail = true;
    await expect(tokens.load()).resolves.toBe(DEV);
  });

  it("does not let the dev token replace a token that was set meanwhile", async () => {
    let release: (token: string) => void = () => undefined;
    const tokens = createTokenStore({
      storage: null,
      dev: true,
      fetchDevToken: () => new Promise<string>((resolve) => (release = resolve)),
    });
    const loading = tokens.load();
    tokens.set(STORED);
    release(DEV);
    expect(await loading).toBe(STORED);
    // Even with storage that does not work, a token that was set is kept over the dev token.
    expect(await tokens.load()).toBe(STORED);
  });

  it("has no token to load without a dev fetcher", async () => {
    expect(await createTokenStore({ storage: null, dev: true }).load()).toBeNull();
  });
});

describe("dev token fetcher", () => {
  it("reads the token from the dev server's endpoint", async () => {
    const { fetch, calls } = fakeFetch({
      [`GET ${DEV_TOKEN_PATH}`]: () => jsonAnswer({ token: DEV }),
    });
    expect(await createDevTokenFetcher(fetch)()).toBe(DEV);
    expect(DEV_TOKEN_PATH).toBe("/__marshal/dev-token");
    expect(calls[0]?.cache).toBe("no-store");
    expect(calls[0]?.url).not.toContain(DEV);
  });

  it.each([
    ["a 404", () => jsonAnswer({ error: "No dev daemon yet." }, 404)],
    ["a 403", () => jsonAnswer({ error: "This machine only." }, 403)],
    ["a body that is not JSON", () => textAnswer("<html>", 200)],
    ["an empty body", () => emptyAnswer(200)],
    ["no token field", () => jsonAnswer({ nope: 1 })],
    ["an empty token", () => jsonAnswer({ token: "" })],
    ["a token that is not text", () => jsonAnswer({ token: 4 })],
  ])("gives no token for %s", async (_name, answer) => {
    const { fetch } = fakeFetch({ [`GET ${DEV_TOKEN_PATH}`]: answer });
    expect(await createDevTokenFetcher(fetch)()).toBeNull();
  });

  it("lets a failed request throw, for the token store to treat as no answer", async () => {
    const { fetch } = fakeFetch({
      [`GET ${DEV_TOKEN_PATH}`]: () => {
        throw new TypeError("Failed to fetch");
      },
    });
    await expect(createDevTokenFetcher(fetch)()).rejects.toThrow("Failed to fetch");
  });

  it("defaults to the page's fetch", async () => {
    const original = globalThis.fetch;
    const { fetch } = fakeFetch({ [`GET ${DEV_TOKEN_PATH}`]: () => jsonAnswer({ token: DEV }) });
    globalThis.fetch = fetch;
    try {
      expect(await createDevTokenFetcher()()).toBe(DEV);
    } finally {
      globalThis.fetch = original;
    }
  });
});
