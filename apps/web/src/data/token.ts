import { type KeyValueStore, readKey, writeKey } from "./storage";

/** Where a paired or signed-in token is kept in `localStorage`. */
export const TOKEN_KEY = "marshal-token";

export interface TokenStoreOptions {
  storage: KeyValueStore | null;
  /** True in the dev build (`import.meta.env.DEV`). Only then may `load` ask for the dev token. */
  dev?: boolean;
  /** Asks the dev server for the dev token. It answers null while the dev daemon has not started. */
  fetchDevToken?: () => Promise<string | null>;
}

export interface TokenStore {
  /** The token to send, or null. It reads storage each time, so another tab's change is seen. */
  get(): string | null;
  /** Keeps a token in storage, and in memory when storage does not work. */
  set(token: string): void;
  /** Forgets the token, in storage and in memory. */
  clear(): void;
  /** Calls the listener when the token changes. Returns a function that stops it. */
  subscribe(listener: () => void): () => void;
  /**
   * Makes sure there is a token if one can be had. A stored token is used as it is. In the dev
   * build with no stored token, it asks the dev server for the dev token every time, because
   * the dev daemon may have started since, or its data folder may have been reset. That token is
   * kept in memory only. It never throws: with no token it returns null.
   */
  load(): Promise<string | null>;
}

interface Held {
  token: string;
  /** True for the dev token, which a later `load` may replace. */
  fromDev: boolean;
}

/**
 * The token of the signed-in device. It is never logged, never shown, and never put in a URL:
 * the API client sends it in a header and the event stream in a subprotocol.
 */
export function createTokenStore({
  storage,
  dev = false,
  fetchDevToken,
}: TokenStoreOptions): TokenStore {
  let held: Held | null = null;
  const listeners = new Set<() => void>();
  const stored = () => readKey(storage, TOKEN_KEY);
  const get = () => stored() ?? held?.token ?? null;
  /** A token from a person or a pairing, which the dev token must never replace. */
  const isFixed = () => stored() !== null || held?.fromDev === false;

  function replace(next: Held | null, write: () => void): void {
    const before = get();
    held = next;
    write();
    if (get() === before) return;
    for (const listener of [...listeners]) listener();
  }

  return {
    get,
    set: (token) => replace({ token, fromDev: false }, () => writeKey(storage, TOKEN_KEY, token)),
    clear: () => replace(null, () => writeKey(storage, TOKEN_KEY, null)),
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    async load() {
      if (isFixed() || !dev || !fetchDevToken) return get();
      let fetched: string | null;
      try {
        fetched = await fetchDevToken();
      } catch {
        // The dev server did not answer, so what is held may still be right.
        return get();
      }
      if (!isFixed()) replace(fetched ? { token: fetched, fromDev: true } : null, () => undefined);
      return get();
    },
  };
}
