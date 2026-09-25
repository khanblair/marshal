import { isRecord } from "./guards";

/** The dev server's own address for the dev token (`apps/web/vite/dev-token.ts`). */
export const DEV_TOKEN_PATH = "/__marshal/dev-token";

/**
 * Asks the dev server for the dev daemon's token. A 404 means the dev daemon has not started, and
 * any other refusal or answer that is not `{ "token": "..." }` counts the same: no token yet. Only
 * a request that fails outright throws, and the token store treats that as no answer.
 */
export function createDevTokenFetcher(
  send: typeof fetch = (input, init) => globalThis.fetch(input, init),
): () => Promise<string | null> {
  return async () => {
    const response = await send(DEV_TOKEN_PATH, { cache: "no-store" });
    if (!response.ok) return null;
    try {
      const body: unknown = await response.json();
      return isRecord(body) && typeof body.token === "string" && body.token !== ""
        ? body.token
        : null;
    } catch {
      return null;
    }
  };
}
