/**
 * A daemon in memory, for the unit and component tests of everything that talks to it. It answers
 * the real API client through a fake `fetch` (so the client, the connection machine, and the event
 * stream that run in a test are the real ones) and pushes events through a fake WebSocket. There is
 * no network and no timer of its own. It knows the routes the cutover uses: health, whoami, the
 * projects, and the agents.
 */
import type { AgentCatalog, Health, Project, WhoAmI } from "@marshal/protocol";
import { vi } from "vitest";
import { createData, type Data } from "~/data";
import { emptyAnswer, errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { type FakeSockets, fakeSockets } from "~/data/testing/fake-web-socket";
import { golden } from "~/data/testing/golden";
import { type MemoryStorage, memoryStorage } from "~/data/testing/memory-storage";
import { TOKEN_KEY } from "~/data/token";
import { wireProject } from "./projects";

/** The token the fake daemon accepts, and the one the page starts with. It is not a real token. */
export const FAKE_TOKEN = "fake-daemon-token-for-tests";
const EPOCH = "01M3C0ZZZZ000000000000000A";
const SOCKET_OPEN = 1;
const MAX_ID_CHARS = 24;
const STATUS = { created: 201, badRequest: 400, unauthorized: 401, notFound: 404, conflict: 409 };

export interface FakeDaemonOptions {
  /** The projects it starts with. */
  projects?: readonly Project[];
  /** The agent catalog it answers with. The golden catalog by default. */
  catalog?: AgentCatalog;
  /** How far its clock is ahead of this device's, in ms. 0 by default. */
  clockSkewMs?: number;
  /** The token the page starts with. `FAKE_TOKEN` by default; null is a page that was never signed in. */
  storedToken?: string | null;
}

export interface FakeDaemon {
  /** The real data layer, wired to this daemon. Give it to the store as `env.data`. */
  data: Data;
  /** The `fetch` it answers, for a test that lets the app's own boot build its data layer. */
  fetch: typeof fetch;
  storage: MemoryStorage;
  sockets: FakeSockets;
  /** Every request the client sent, in order. */
  calls: FakeRequest[];
  /** The projects it holds now. A test reads them, and changes them with the calls or `emit`. */
  projects: Project[];
  catalog: AgentCatalog;
  /** Makes every call fail like a daemon that is not running, until `start`. */
  stop(): void;
  start(): void;
  /** Makes the daemon refuse the token from now on, like one that was revoked. */
  revokeToken(): void;
  /** Answers the next call to `"METHOD /path"` with this error, once. */
  refuseNext(route: string, status: number, code: string, message: string): void;
  /** Holds the answer of the next call to `"METHOD /path"` until the returned function is called. */
  holdNext(route: string): () => void;
  /** Waits for the connection to be online, opens its event stream, and greets it as the daemon does. */
  connect(): Promise<void>;
  /** Sends one event on the open stream, as the daemon does after a change. */
  emit(topic: string, type: string, data: unknown): void;
  /** The paths (with the method) of the calls made so far, for a short assertion. */
  routes(): string[];
  /** The JSON bodies sent to `"METHOD /path"`, in order. A call with no body is not listed. */
  bodies(route: string): unknown[];
}

const pathOf = (url: string): string => url.replace(/^https?:\/\/[^/]+/, "");
const keyOf = (request: FakeRequest): string => `${request.method} ${pathOf(request.url)}`;

const slug = (name: string): string =>
  name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, MAX_ID_CHARS) || "project";

const baseName = (path: string): string => path.replace(/\/+$/, "").split("/").pop() ?? "";

/** Reads the JSON body of a request. A body that is not JSON is `{}`, which the routes then refuse. */
function bodyOf(request: FakeRequest): Record<string, unknown> {
  try {
    return request.body ? (JSON.parse(request.body) as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

/** Tells the event stream about a change, as the daemon does after it. */
type Publish = (topic: string, type: string, data: unknown) => void;

const refuse = (status: number, code: string, message: string) =>
  errorAnswer(status, code, message);

interface ProjectState {
  projects: Project[];
  publish: Publish;
  now: () => string;
}

const notFound = () =>
  refuse(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that project. It may have been removed.",
  );

function createProject({ projects, publish, now }: ProjectState, request: FakeRequest): Response {
  const body = bodyOf(request);
  const path = String(body.path ?? body.dest ?? "").trim();
  if (!path)
    return refuse(STATUS.badRequest, "invalid_argument", "Choose the folder of the repository.");
  if (projects.some((p) => p.path === path)) {
    return refuse(STATUS.conflict, "conflict", "That repository is already a project in Marshal.");
  }
  const name = String(body.name ?? "").trim() || baseName(path);
  const project = wireProject({
    id: slug(name),
    name,
    path,
    language: "Unknown",
    devCommand: "",
    bypassLocked: false,
    badges: { needs: 0, awake: 0 },
    createdAt: now(),
  });
  projects.push(project);
  publish("home", "project.created", { project });
  return jsonAnswer(project, STATUS.created);
}

function updateProject(
  { projects, publish }: ProjectState,
  id: string,
  request: FakeRequest,
): Response {
  const project = projects.find((p) => p.id === id);
  if (!project) return notFound();
  const body = bodyOf(request);
  if (typeof body.name === "string") {
    if (!body.name.trim()) {
      return refuse(
        STATUS.badRequest,
        "invalid_argument",
        "Project names can't be empty. The old name is kept.",
      );
    }
    project.name = body.name.trim();
  }
  if (typeof body.devCommand === "string") project.devCommand = body.devCommand;
  if (typeof body.defaultBranch === "string") project.defaultBranch = body.defaultBranch;
  if (typeof body.bypassLocked === "boolean") project.bypassLocked = body.bypassLocked;
  publish("home", "project.updated", { project });
  return jsonAnswer(project);
}

function removeProject({ projects, publish }: ProjectState, id: string): Response {
  const at = projects.findIndex((p) => p.id === id);
  if (at < 0) return notFound();
  projects.splice(at, 1);
  publish("home", "project.removed", { projectId: id });
  return emptyAnswer();
}

interface Router {
  /** The token the daemon holds as the right one, or null once it was revoked. */
  token: () => string | null;
  now: () => string;
  projects: Project[];
  state: ProjectState;
  catalog: AgentCatalog;
}

/** Answers one request the way the real daemon's router does, including who may ask. */
function answer(router: Router, request: FakeRequest): Response {
  const key = keyOf(request);
  // The dev server's own route, which the dev build asks for the token.
  if (key === "GET /__marshal/dev-token") return jsonAnswer({ token: FAKE_TOKEN });
  if (key === "GET /v1/health") {
    return jsonAnswer({ ...golden<Health>("health"), serverTime: router.now() });
  }
  if (request.headers.authorization !== `Bearer ${router.token()}`) {
    return refuse(
      STATUS.unauthorized,
      "unauthorized",
      "Sign in again. This device's token is missing or no longer valid.",
    );
  }
  const id = /^\/v1\/projects\/([^/]+)$/.exec(pathOf(request.url))?.[1] ?? "";
  if (key === "GET /v1/auth/whoami") {
    return jsonAnswer({ ...golden<WhoAmI>("whoami"), serverTime: router.now() });
  }
  if (key === "GET /v1/projects") {
    return jsonAnswer({ projects: router.projects, serverTime: router.now() });
  }
  if (key === "POST /v1/projects") return createProject(router.state, request);
  if (request.method === "PATCH" && id) return updateProject(router.state, id, request);
  if (request.method === "DELETE" && id) return removeProject(router.state, id);
  if (key === "GET /v1/agents" || key === "POST /v1/agents/refresh") {
    return jsonAnswer({ ...router.catalog, serverTime: router.now() });
  }
  return refuse(STATUS.notFound, "not_found", "Marshal has nothing at that address.");
}

function toRequest(input: RequestInfo | URL, init?: RequestInit): FakeRequest {
  const headers: Record<string, string> = {};
  new Headers(init?.headers).forEach((value, name) => {
    headers[name] = value;
  });
  return {
    url: String(input),
    method: init?.method ?? "GET",
    headers,
    body: typeof init?.body === "string" ? init.body : null,
    signal: init?.signal ?? null,
    credentials: init?.credentials,
    cache: init?.cache,
  };
}

/** What a test can make the daemon do to the next call to one route. */
interface Interference {
  refusals: Map<string, Response>;
  holds: Map<string, Promise<void>>;
}

function interference(): Interference & Pick<FakeDaemon, "refuseNext" | "holdNext"> {
  const refusals = new Map<string, Response>();
  const holds = new Map<string, Promise<void>>();
  return {
    refusals,
    holds,
    refuseNext: (route, status, code, message) => {
      refusals.set(route, errorAnswer(status, code, message));
    },
    holdNext: (route) => {
      let release = () => {};
      holds.set(
        route,
        new Promise<void>((resolve) => {
          release = () => {
            holds.delete(route);
            resolve();
          };
        }),
      );
      return release;
    },
  };
}

interface DaemonState {
  running: boolean;
  token: string | null;
  seq: number;
}

/** What a test can flip about the daemon as a whole: running or not, and whether its token is still good. */
function switches(state: DaemonState): Pick<FakeDaemon, "stop" | "start" | "revokeToken"> {
  return {
    stop: () => {
      state.running = false;
    },
    start: () => {
      state.running = true;
    },
    revokeToken: () => {
      state.token = null;
    },
  };
}

/** Waits until the connection is online, then accepts its event stream and sends the Resync every new connection gets. */
async function greetStream(data: Data, sockets: FakeSockets): Promise<void> {
  await vi.waitFor(() => {
    if (data.connection.state() !== "online") throw new Error("not online yet");
    if (sockets.all.length === 0) throw new Error("no stream yet");
  });
  const socket = sockets.last();
  socket.accept();
  socket.push({ type: "resync", epoch: EPOCH, reason: "epoch-changed", seq: 0 });
}

/** Sends events to the newest socket, as the daemon does, once it is open. */
function publisher(sockets: FakeSockets, state: DaemonState, now: () => string): Publish {
  return (topic, type, data) => {
    const socket = sockets.all.at(-1);
    if (!socket) return;
    if (socket.readyState !== SOCKET_OPEN) return;
    state.seq += 1;
    socket.push({
      type: "events",
      epoch: EPOCH,
      events: [{ seq: state.seq, topic, type, at: now(), data }],
    });
  };
}

/** The `fetch` of the fake daemon: records each call, waits or refuses when a test said so, else answers. */
function fakeFetchOf(
  state: DaemonState,
  calls: FakeRequest[],
  meddle: Interference,
  router: Router,
): typeof fetch {
  return (input, init) => {
    const request = toRequest(input, init);
    calls.push(request);
    return (async () => {
      const key = keyOf(request);
      await meddle.holds.get(key);
      if (!state.running) throw new TypeError("Failed to fetch");
      const refusal = meddle.refusals.get(key);
      meddle.refusals.delete(key);
      return refusal ?? answer(router, request);
    })();
  };
}

export function createFakeDaemon(options: FakeDaemonOptions = {}): FakeDaemon {
  const now = (): string => new Date(Date.now() + (options.clockSkewMs ?? 0)).toISOString();
  const stored = options.storedToken === undefined ? FAKE_TOKEN : options.storedToken;
  const storage = memoryStorage(stored === null ? {} : { [TOKEN_KEY]: stored });
  const sockets = fakeSockets();
  const state: DaemonState = { running: true, token: FAKE_TOKEN, seq: 0 };
  const calls: FakeRequest[] = [];
  const meddle = interference();
  // A copy, so a test that changes a project through the daemon does not change the shared fixture.
  const projects = structuredClone([...(options.projects ?? [])]);
  const catalog = options.catalog ?? golden<AgentCatalog>("agents");

  const emit = publisher(sockets, state, now);
  const router: Router = {
    token: () => state.token,
    now,
    projects,
    state: { projects, publish: emit, now },
    catalog,
  };

  const fake = fakeFetchOf(state, calls, meddle, router);
  const data = createData({
    baseUrl: "",
    storage,
    fetch: fake,
    WebSocketImpl: sockets.Impl,
    page: { protocol: "http:", host: "localhost:3210" },
    watchPage: () => () => undefined,
  });

  return {
    data,
    fetch: fake,
    storage,
    sockets,
    calls,
    projects,
    catalog,
    ...switches(state),
    refuseNext: meddle.refuseNext,
    holdNext: meddle.holdNext,
    connect: () => greetStream(data, sockets),
    emit,
    routes: () => calls.map(keyOf),
    bodies: (route) =>
      calls
        .filter((call) => keyOf(call) === route && call.body !== null)
        .map((call) => JSON.parse(call.body ?? "null")),
  };
}
