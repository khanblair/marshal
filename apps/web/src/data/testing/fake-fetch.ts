const NOT_FOUND = 404;

/** One request as the fake `fetch` saw it. Header names are lower case. */
export interface FakeRequest {
  url: string;
  method: string;
  headers: Record<string, string>;
  body: string | null;
  /** The body of a request that is not text, such as an uploaded image. */
  raw?: BodyInit | null;
  signal: AbortSignal | null;
  credentials: RequestCredentials | undefined;
  cache: RequestCache | undefined;
}

type Answer = Response | Promise<Response>;
export type FakeHandler = (request: FakeRequest) => Answer;

export interface FakeFetch {
  fetch: typeof fetch;
  calls: FakeRequest[];
}

/** A JSON answer, like the daemon's. */
export function jsonAnswer(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json; charset=utf-8" },
  });
}

/** An answer with no body, such as the 204 of a stop. */
export function emptyAnswer(status = 204): Response {
  return new Response(null, { status });
}

/** An answer with a text body that is not JSON, such as a proxy's error page. */
export function textAnswer(body: string, status: number): Response {
  return new Response(body, { status, headers: { "Content-Type": "text/plain" } });
}

/** The daemon's error answer. */
export function errorAnswer(status: number, code: string, message: string): Response {
  return jsonAnswer({ error: { code, message } }, status);
}

/** A request that never gets an answer until it is cancelled. */
export function hang(): Promise<Response> {
  return new Promise<Response>(() => undefined);
}

function readHeaders(init: RequestInit | undefined): Record<string, string> {
  const headers: Record<string, string> = {};
  new Headers(init?.headers).forEach((value, key) => {
    headers[key] = value;
  });
  return headers;
}

/**
 * A `fetch` that answers from a route table, keyed by `"METHOD /path"`, and records every call.
 * A handler may throw to act like a network failure. A request that is cancelled rejects the way
 * the real one does, even when its handler never answers.
 */
export function fakeFetch(routes: Record<string, FakeHandler>): FakeFetch {
  const calls: FakeRequest[] = [];
  const fake = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const request: FakeRequest = {
      url: String(input),
      method: init?.method ?? "GET",
      headers: readHeaders(init),
      body: typeof init?.body === "string" ? init.body : null,
      raw: typeof init?.body === "string" ? null : (init?.body ?? null),
      signal: init?.signal ?? null,
      credentials: init?.credentials,
      cache: init?.cache,
    };
    calls.push(request);
    return new Promise<Response>((resolve, reject) => {
      const abort = () => reject(new DOMException("The operation was aborted.", "AbortError"));
      if (request.signal?.aborted) return abort();
      request.signal?.addEventListener("abort", abort, { once: true });
      const path = request.url.replace(/^https?:\/\/[^/]+/, "");
      const handler = routes[`${request.method} ${path}`];
      if (!handler)
        return resolve(errorAnswer(NOT_FOUND, "not_found", "Marshal has nothing at that address."));
      // Not `resolve(handler(request))`: that would lock the promise to a handler that hangs, and a
      // cancel could no longer reject it.
      try {
        Promise.resolve(handler(request)).then(resolve, reject);
      } catch (error) {
        reject(error);
      }
    });
  };
  return { fetch: fake, calls };
}
