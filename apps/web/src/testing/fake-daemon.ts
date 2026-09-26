/**
 * A daemon in memory, for the unit and component tests of everything that talks to it. It answers
 * the real API client through a fake `fetch` (so the client, the connection machine, and the event
 * stream that run in a test are the real ones) and pushes events through a fake WebSocket. There is
 * no network and no timer of its own. It knows the routes the cutover uses: health, whoami, the
 * projects, and the agents.
 */
import type {
  AgentCatalog,
  Card,
  Chat,
  FeedEntry,
  Health,
  Label,
  Preferences,
  Profile,
  Progress,
  Project,
  SavedView,
  TerminalInput,
  TerminalResize,
  WhoAmI,
} from "@marshal/protocol";
import { vi } from "vitest";
import { createData, type Data } from "~/data";
import { emptyAnswer, errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { type FakeSockets, fakeSockets } from "~/data/testing/fake-web-socket";
import { golden } from "~/data/testing/golden";
import { type MemoryStorage, memoryStorage } from "~/data/testing/memory-storage";
import { TOKEN_KEY } from "~/data/token";
import { answerCardRoute, type CardStore, type FakeCardDiff, type HistoryRow } from "./fake-cards";
import { answerChatRoute, type ChatMessageRow, type ChatStore } from "./fake-chats";
import {
  answerMeRoute,
  emptyPreferences,
  type MeStore,
  pendingProgress,
  wireProfile,
} from "./fake-me";
import { answerSavedViewRoute } from "./fake-saved-views";
import { answerSearchRoute } from "./fake-search";
import { createTerminalRouter, type TerminalRouter } from "./fake-terminal";
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
  /** The cards it starts with, across all projects. None by default: the boards are empty. */
  cards?: readonly Card[];
  /** The labels it starts with, across all projects. None by default. */
  labels?: readonly Label[];
  /** The stored history it starts with: a card's chat and activity, newest first per card. */
  history?: readonly HistoryRow[];
  /** The project chats it starts with, across all projects. None by default. */
  chats?: readonly Chat[];
  /** The stored messages of those chats it starts with. None by default. */
  chatMessages?: readonly ChatMessageRow[];
  /** What a chat's agent answers to a message. A plain sentence that repeats the message by default. */
  chatAnswer?: (text: string) => string;
  /** The Home activity stream it starts with (section S20), newest first. None by default. */
  activity?: readonly FeedEntry[];
  /** Every card's diff it starts with (section S11), by card id. None by default. */
  diffs?: Readonly<Record<string, FakeCardDiff>>;
  /** The person it starts with (sections S2a, S32, S31a). The golden profile with no avatar by default. */
  profile?: Profile;
  /** The person's preferences it starts with. The defaults by default: nothing saved. */
  preferences?: Preferences;
  /** The person's onboarding and tour progress it starts with. Both pending by default. */
  progress?: Progress;
  /** The saved views it starts with, across projects (section S6a). None by default. */
  savedViews?: readonly SavedView[];
  /** True to run in dev mode, which is what the first-launch reset route needs. False by default. */
  dev?: boolean;
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
  /** Every card it holds now, across projects. A test reads them, and adds to them. */
  cards: Card[];
  /** Every label it holds now, across projects, for the label routes and `label.updated`. */
  labels: Label[];
  /** Every stored history row it holds now: the chat and activity it serves for each card. */
  history: HistoryRow[];
  /** Every chat it holds now, across projects, for the chat routes and the `chat.*` events. */
  chats: Chat[];
  /** Every stored chat message it holds now, for a chat's history and its detail route. */
  chatMessages: ChatMessageRow[];
  /** Every row of the Home activity stream it holds now, newest first. */
  activity: FeedEntry[];
  /** Every card's diff it holds now, by card id. */
  diffs: Record<string, FakeCardDiff>;
  /** The person it holds now: the profile, the progress, the preferences, and every saved view. */
  me: MeStore;
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
  /**
   * A second device on the same daemon: its own token store and connection, and events reach it as
   * they reach the first. Start it by giving its `data` to a store, then `connectAnother`.
   */
  another(): Data;
  /** Like `connect`, for a device made by `another`. */
  connectAnother(data: Data): Promise<void>;
  /** Sends one event on the open stream, as the daemon does after a change. */
  emit(topic: string, type: string, data: unknown): void;
  /** The paths (with the method) of the calls made so far, for a short assertion. */
  routes(): string[];
  /** The JSON bodies sent to `"METHOD /path"`, in order. A call with no body is not listed. */
  bodies(route: string): unknown[];
  /** Every `terminal.input` a card's terminal received over the event stream, in order (section S9). */
  terminalInputs(cardId: string): readonly TerminalInput[];
  /** Every `terminal.resize` a card's terminal received, in order. */
  terminalResizes(cardId: string): readonly TerminalResize[];
  /** Publishes a piece of a card's terminal output as `session.terminal_output`, live-only like
   * the daemon's own, and keeps it in the card's screen for the next `terminal.snapshot`. */
  emitTerminalOutput(cardId: string, text: string): void;
  /** Makes the next terminal message about a card answer `terminal.refused` (`terminal_not_active`),
   * or restores it. Every card's terminal starts active. */
  setTerminalActive(cardId: string, active: boolean): void;
  /** Makes `terminal.input` about a card answer `terminal.refused` (`terminal_busy`), or restores it. */
  setTerminalBusy(cardId: string, busy: boolean): void;
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

/** The sample ships inside the daemon: one folder, so the request names only the source. */
const SAMPLE_NAME = "marshal-sample";
const SAMPLE_PATH = "~/.marshal/sample";

function createSampleProject(
  { projects, publish, now }: ProjectState,
  body: Record<string, unknown>,
): Response {
  const already = projects.find((p) => p.name === SAMPLE_NAME || p.path === SAMPLE_PATH);
  if (already) {
    return refuse(STATUS.conflict, "conflict", "The sample project is already in Marshal.");
  }
  const name = String(body.name ?? "").trim() || SAMPLE_NAME;
  const project = wireProject({
    id: slug(name),
    name,
    path: SAMPLE_PATH,
    language: "TypeScript",
    devCommand: "",
    bypassLocked: false,
    badges: { needs: 0, awake: 0 },
    createdAt: now(),
  });
  projects.push(project);
  publish("home", "project.created", { project });
  return jsonAnswer(project, STATUS.created);
}

function createProject({ projects, publish, now }: ProjectState, request: FakeRequest): Response {
  const body = bodyOf(request);
  if (body.source === "sample") return createSampleProject({ projects, publish, now }, body);
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
  cards: CardStore;
  chats: ChatStore;
  me: MeStore;
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
  const cards = answerCardRoute(
    router.cards,
    request,
    (pid) => router.projects.some((p) => p.id === pid),
    (pid) => router.projects.find((p) => p.id === pid)?.name ?? pid,
  );
  if (cards) return cards;
  const chat = answerChatRoute(router.chats, request, (pid) =>
    router.projects.some((p) => p.id === pid),
  );
  if (chat) return chat;
  const projectExists = (pid: string) => router.projects.some((p) => p.id === pid);
  const person = answerMeRoute(router.me, request, projectExists);
  if (person) return person;
  const views = answerSavedViewRoute(router.me, request, projectExists);
  if (views) return views;
  const found = answerSearchRoute(
    {
      projects: router.projects,
      cards: router.cards.cards,
      chats: router.chats.chats,
      now: router.now,
    },
    request,
  );
  if (found) return found;
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
    raw: typeof init?.body === "string" ? null : (init?.body ?? null),
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

/**
 * Waits until the connection is online and has opened a stream after the first `already` ones, then
 * accepts that stream and sends the Resync every new connection gets. The first device opens the
 * first stream; a device that joins later (`another`) opens the next.
 */
async function greetStream(data: Data, sockets: FakeSockets, already = 0): Promise<void> {
  await vi.waitFor(() => {
    if (data.connection.state() !== "online") throw new Error("not online yet");
    if (sockets.all.length <= already) throw new Error("no stream yet");
  });
  const socket = sockets.all[already] ?? sockets.last();
  socket.accept();
  socket.push({ type: "resync", epoch: EPOCH, reason: "epoch-changed", seq: 0 });
}

/** Sends events to every stream that is open, as the daemon does to each device that follows. */
function publisher(sockets: FakeSockets, state: DaemonState, now: () => string): Publish {
  return (topic, type, data) => {
    const open = sockets.all.filter((socket) => socket.readyState === SOCKET_OPEN);
    if (open.length === 0) return;
    state.seq += 1;
    for (const socket of open) {
      socket.push({
        type: "events",
        epoch: EPOCH,
        events: [{ seq: state.seq, topic, type, at: now(), data }],
      });
    }
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
  // The terminal router needs the card store and the publisher, neither of which exists yet at the
  // point the sockets are made (the publisher needs the sockets first), so it reaches it through a
  // ref that is filled in once both do (the same shape `data/index.ts` uses for its own circular
  // need between the connection and the stream).
  const termRef: { current: TerminalRouter | null } = { current: null };
  const sockets = fakeSockets({ onSend: (socket, data) => termRef.current?.onSend(socket, data) });
  const state: DaemonState = { running: true, token: FAKE_TOKEN, seq: 0 };
  const calls: FakeRequest[] = [];
  const meddle = interference();
  // A copy, so a test that changes a project through the daemon does not change the shared fixture.
  const projects = structuredClone([...(options.projects ?? [])]);
  const catalog = options.catalog ?? golden<AgentCatalog>("agents");
  const cards = structuredClone([...(options.cards ?? [])]);
  const history = structuredClone([...(options.history ?? [])]);
  const labels = structuredClone([...(options.labels ?? [])]);
  const chats = structuredClone([...(options.chats ?? [])]);
  const chatMessages = structuredClone([...(options.chatMessages ?? [])]);
  const activity = structuredClone([...(options.activity ?? [])]);
  const diffs = structuredClone(options.diffs ?? {});

  const emit = publisher(sockets, state, now);
  const terminals = createTerminalRouter({ publish: emit, seqNow: () => state.seq });
  termRef.current = terminals;
  const profile = structuredClone(options.profile ?? wireProfile());
  const me: MeStore = {
    profile,
    progress: structuredClone(options.progress ?? pendingProgress()),
    preferences: structuredClone(options.preferences ?? emptyPreferences()),
    savedViews: structuredClone([...(options.savedViews ?? [])]),
    // A profile that starts with an avatar has one to serve.
    avatar: profile.avatarUrl ? { type: "image/png", size: 70 } : null,
    dev: options.dev ?? false,
    publish: emit,
    now,
  };
  const router: Router = {
    token: () => state.token,
    now,
    projects,
    state: { projects, publish: emit, now },
    catalog,
    cards: { cards, labels, history, activity, diffs, publish: emit, now },
    chats: { chats, messages: chatMessages, answer: options.chatAnswer, publish: emit, now },
    me,
  };

  const fake = fakeFetchOf(state, calls, meddle, router);
  const makeData = (kept: MemoryStorage): Data =>
    createData({
      baseUrl: "",
      storage: kept,
      fetch: fake,
      WebSocketImpl: sockets.Impl,
      page: { protocol: "http:", host: "localhost:3210" },
      watchPage: () => () => undefined,
    });
  const data = makeData(storage);

  return {
    data,
    fetch: fake,
    storage,
    sockets,
    calls,
    projects,
    catalog,
    cards,
    labels,
    history,
    chats,
    chatMessages,
    activity,
    diffs,
    me,
    ...switches(state),
    refuseNext: meddle.refuseNext,
    holdNext: meddle.holdNext,
    connect: () => greetStream(data, sockets),
    // Its own token store, so stopping one device does not touch the other.
    another: () => makeData(memoryStorage(stored === null ? {} : { [TOKEN_KEY]: stored })),
    connectAnother: (other) => greetStream(other, sockets, sockets.all.length),
    emit,
    routes: () => calls.map(keyOf),
    bodies: (route) =>
      calls
        .filter((call) => keyOf(call) === route && call.body !== null)
        .map((call) => JSON.parse(call.body ?? "null")),
    terminalInputs: (cardId) => terminals.inputsOf(cardId),
    terminalResizes: (cardId) => terminals.resizesOf(cardId),
    emitTerminalOutput: (cardId, text) => terminals.emitOutput(cardId, text),
    setTerminalActive: (cardId, active) => terminals.setActive(cardId, active),
    setTerminalBusy: (cardId, busy) => terminals.setBusy(cardId, busy),
  };
}
