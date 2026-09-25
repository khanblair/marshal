import type {
  AgentCatalog,
  BoardSnapshot,
  Card,
  CreateCardRequest,
  CreateProjectRequest,
  Health,
  Project,
  ProjectListSnapshot,
  RemoveProjectRequest,
  SendMessageRequest,
  UpdateProjectRequest,
  WhoAmI,
} from "@marshal/protocol";
import { type ApiError, clientError, readWireError, wireError } from "./api-error";
import type { DaemonClock } from "./daemon-clock";
import { isRecord } from "./guards";
import { guardRequest } from "./request-guard";

type Method = "GET" | "POST" | "PATCH" | "DELETE";

const DEFAULT_TIMEOUT_MS = 30_000;
/** Starting or resuming a card makes a worktree and starts a program, so the daemon may take minutes. */
const SLOW_TIMEOUT_MS = 180_000;
const UNAUTHORIZED = 401;

/** Options of one call. */
interface CallOptions {
  /** Cancels the request. The call then fails with the code `aborted`. */
  signal?: AbortSignal | undefined;
  /** Replaces the time limit of the client for this call. */
  timeoutMs?: number | undefined;
}

interface RequestOptions extends CallOptions {
  /** Sent as JSON. Leave it out for a request with no body. */
  body?: unknown;
}

export interface ApiClientOptions {
  /** The daemon's address, such as `http://127.0.0.1:47800`. Empty means the address of the page. */
  baseUrl?: string;
  /** The token to send. With none, the request is still sent, and the daemon answers 401. */
  getToken: () => string | null;
  fetch?: typeof fetch;
  /** Learns the daemon's clock from every answer that has a `serverTime`. */
  clock?: Pick<DaemonClock, "observe">;
  /** The time limit of a call, in ms. Starting and resuming a card have their own, longer one. */
  timeoutMs?: number;
  /** Called once for every 401 answer, before the error is thrown. */
  onUnauthorized?: (error: ApiError) => void;
  /** The local clock in ms. It is only for the clock offset, and tests replace it. */
  localNow?: () => number;
}

/** One typed method for each route of the daemon (architecture.md section 11.1). */
export interface ApiClient {
  /** The path every method below is built on. Use it for a route that has no method yet. */
  request<T>(method: Method, path: string, options?: RequestOptions): Promise<T>;
  health(options?: CallOptions): Promise<Health>;
  whoami(options?: CallOptions): Promise<WhoAmI>;
  listProjects(options?: CallOptions): Promise<ProjectListSnapshot>;
  createProject(body: CreateProjectRequest, options?: CallOptions): Promise<Project>;
  getProject(id: string, options?: CallOptions): Promise<Project>;
  updateProject(id: string, body: UpdateProjectRequest, options?: CallOptions): Promise<Project>;
  removeProject(id: string, body?: RemoveProjectRequest, options?: CallOptions): Promise<void>;
  board(projectId: string, options?: CallOptions): Promise<BoardSnapshot>;
  createCard(projectId: string, body: CreateCardRequest, options?: CallOptions): Promise<Card>;
  getCard(id: string, options?: CallOptions): Promise<Card>;
  startCard(id: string, options?: CallOptions): Promise<Card>;
  sendMessage(id: string, body: SendMessageRequest, options?: CallOptions): Promise<void>;
  stopCard(id: string, options?: CallOptions): Promise<void>;
  resumeCard(id: string, options?: CallOptions): Promise<void>;
  agents(options?: CallOptions): Promise<AgentCatalog>;
  refreshAgents(options?: CallOptions): Promise<AgentCatalog>;
}

function parseJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

/**
 * A proxy in front of the daemon answers with these when the daemon is not there, and it does not
 * use the daemon's error shape. The dev server does so when the dev daemon is down. The daemon's
 * own 503 has the error shape and is read as `unavailable`.
 */
const BAD_GATEWAY = 502;
const SERVICE_UNAVAILABLE = 503;
const GATEWAY_TIMEOUT = 504;
const GATEWAY_STATUSES: ReadonlySet<number> = new Set([
  BAD_GATEWAY,
  SERVICE_UNAVAILABLE,
  GATEWAY_TIMEOUT,
]);

/**
 * The error for an answer that is not a success: the daemon's own, no daemon behind a proxy, or
 * an answer that is not the daemon's, with its status.
 */
function failure(status: number, text: string): ApiError {
  const body = parseJson(text);
  const wire = readWireError(isRecord(body) ? body.error : undefined);
  if (wire) return wireError(status, wire);
  return clientError(GATEWAY_STATUSES.has(status) ? "unreachable" : "bad_response", status);
}

interface Transport {
  request<T>(method: Method, path: string, options?: RequestOptions): Promise<T>;
  command(method: Method, path: string, options?: RequestOptions): Promise<void>;
}

interface Received {
  ok: boolean;
  status: number;
  text: string;
  receivedAt: number;
}

/** The part that talks over the network: headers, time limits, and the way each call can fail. */
function createTransport(options: ApiClientOptions): Transport {
  const { getToken, clock, onUnauthorized, localNow = Date.now } = options;
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const root = (options.baseUrl ?? "").replace(/\/+$/, "");
  const send = options.fetch ?? ((input, init) => globalThis.fetch(input, init));

  async function receive(method: Method, path: string, o: RequestOptions): Promise<Received> {
    const guard = guardRequest(o.signal, o.timeoutMs ?? timeoutMs);
    const token = getToken();
    const headers: Record<string, string> = { Accept: "application/json" };
    if (token) headers.Authorization = `Bearer ${token}`;
    if (o.body !== undefined) headers["Content-Type"] = "application/json";
    try {
      const response = await send(root + path, {
        method,
        headers,
        body: o.body === undefined ? null : JSON.stringify(o.body),
        signal: guard.signal,
        credentials: "omit",
        cache: "no-store",
      });
      const receivedAt = localNow();
      return { ok: response.ok, status: response.status, text: await response.text(), receivedAt };
    } catch {
      throw clientError(guard.cause() ?? "unreachable");
    } finally {
      guard.release();
    }
  }

  /** Sends one request and returns the text of a success. Throws an `ApiError` for anything else. */
  async function exchange(method: Method, path: string, o: RequestOptions) {
    const sentAt = localNow();
    const answer = await receive(method, path, o);
    if (!answer.ok) {
      const error = failure(answer.status, answer.text);
      if (answer.status === UNAUTHORIZED) onUnauthorized?.(error);
      throw error;
    }
    return { text: answer.text, sentAt, receivedAt: answer.receivedAt };
  }

  return {
    async request<T>(method: Method, path: string, o: RequestOptions = {}): Promise<T> {
      const { text, sentAt, receivedAt } = await exchange(method, path, o);
      const body = parseJson(text);
      if (body === undefined) throw clientError("bad_response");
      if (isRecord(body) && typeof body.serverTime === "string") {
        clock?.observe({ serverTime: body.serverTime, sentAt, receivedAt });
      }
      // The daemon's types are generated from the Go ones and checked against golden files, so the
      // shape is trusted here instead of being checked again on every answer.
      return body as T;
    },
    async command(method, path, o = {}) {
      await exchange(method, path, o);
    },
  };
}

const slow = (o: CallOptions | undefined): CallOptions => ({
  ...o,
  timeoutMs: o?.timeoutMs ?? SLOW_TIMEOUT_MS,
});

/** The typed method of each route. Path segments are escaped, and a call with no body sends none. */
function routeMethods({ request, command }: Transport): Omit<ApiClient, "request"> {
  const id = encodeURIComponent;
  return {
    health: (o) => request("GET", "/v1/health", o),
    whoami: (o) => request("GET", "/v1/auth/whoami", o),
    listProjects: (o) => request("GET", "/v1/projects", o),
    createProject: (body, o) => request("POST", "/v1/projects", { ...o, body }),
    getProject: (pid, o) => request("GET", `/v1/projects/${id(pid)}`, o),
    updateProject: (pid, body, o) => request("PATCH", `/v1/projects/${id(pid)}`, { ...o, body }),
    removeProject: (pid, body, o) => command("DELETE", `/v1/projects/${id(pid)}`, { ...o, body }),
    board: (pid, o) => request("GET", `/v1/projects/${id(pid)}/board`, o),
    createCard: (pid, body, o) => request("POST", `/v1/projects/${id(pid)}/cards`, { ...o, body }),
    getCard: (cid, o) => request("GET", `/v1/cards/${id(cid)}`, o),
    startCard: (cid, o) => request("POST", `/v1/cards/${id(cid)}/start`, slow(o)),
    sendMessage: (cid, body, o) => command("POST", `/v1/cards/${id(cid)}/messages`, { ...o, body }),
    stopCard: (cid, o) => command("POST", `/v1/cards/${id(cid)}/stop`, o),
    resumeCard: (cid, o) => command("POST", `/v1/cards/${id(cid)}/resume`, slow(o)),
    agents: (o) => request("GET", "/v1/agents", o),
    refreshAgents: (o) => request("POST", "/v1/agents/refresh", o),
  };
}

/**
 * The one client for every call to the daemon. It sends the token, times each call out, tells
 * apart the ways a call can fail (`ApiError`), and keeps the daemon's clock. It never logs a
 * request or an answer.
 */
export function createApiClient(options: ApiClientOptions): ApiClient {
  const transport = createTransport(options);
  return { request: transport.request, ...routeMethods(transport) };
}
