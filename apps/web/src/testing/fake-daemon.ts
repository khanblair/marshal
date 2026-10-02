/**
 * A daemon in memory, for the unit and component tests of everything that talks to it. It answers
 * the real API client through a fake `fetch` (so the client, the connection machine, and the event
 * stream that run in a test are the real ones) and pushes events through a fake WebSocket. There is
 * no network and no timer of its own. It knows the routes the cutover uses: health, whoami, the
 * projects, the agents, the providers, the connections, the limits, the roles, the notices, the sleep
 * settings, every project's CI health, and a card's live preview.
 */
import type {
  AgentCatalog,
  AlertSettings,
  Card,
  Chat,
  Checkpoint,
  FeedEntry,
  Health,
  Integration,
  IntegrationState,
  Label,
  Limit,
  Note,
  Notice,
  Preferences,
  Preview,
  Profile,
  Progress,
  Project,
  ProjectCI,
  Provider,
  Role,
  SavedView,
  Schedule,
  SleepSettings,
  TerminalInput,
  TerminalResize,
  TestCheck,
  WhoAmI,
} from "@marshal/protocol";
import { vi } from "vitest";
import { createData, type Data } from "~/data";
import { emptyAnswer, errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { type FakeSockets, fakeSockets } from "~/data/testing/fake-web-socket";
import { golden } from "~/data/testing/golden";
import { type MemoryStorage, memoryStorage } from "~/data/testing/memory-storage";
import { TOKEN_KEY } from "~/data/token";
import { type AlertsStore, answerAlertsRoute, createAlertsStore } from "./fake-alerts";
import { answerCardPanelRoute, type CardPanelStore, createCardPanelStore } from "./fake-card-panel";
import { answerCardRoute, type CardStore, type FakeCardDiff, type HistoryRow } from "./fake-cards";
import { answerChatRoute, type ChatMessageRow, type ChatStore } from "./fake-chats";
import { answerCIRoute, type CIStore, createCIStore } from "./fake-ci";
import {
  answerIntegrationRoute,
  createIntegrationStore,
  type IntegrationStore,
} from "./fake-integrations";
import { answerLimitRoute, createLimitsStore, type LimitsStore } from "./fake-limits";
import {
  answerMeRoute,
  emptyPreferences,
  type MeStore,
  pendingProgress,
  wireProfile,
} from "./fake-me";
import { answerMergeFlowRoute, createMergeFlowStore, type MergeFlowStore } from "./fake-merge-flow";
import { answerNoticeRoute, createNoticesStore, type NoticesStore } from "./fake-notices";
import { createPreviewStore, type PreviewStore } from "./fake-previews";
import { answerProviderRoute, createProviderStore, type ProviderStore } from "./fake-providers";
import { answerRoleRoute, createRolesStore, type RolesStore } from "./fake-roles";
import { answerSavedViewRoute } from "./fake-saved-views";
import {
  answerCalendarRoute,
  answerScheduleRoute,
  createScheduleStore,
  type ScheduleStore,
} from "./fake-schedules";
import { answerSearchRoute } from "./fake-search";
import { answerSleepRoute, createSleepStore, type SleepStore } from "./fake-sleep";
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
  /** Every card's restore points it starts with (B5.3), by card id, newest first. None by default. */
  checkpoints?: Readonly<Record<string, readonly Checkpoint[]>>;
  /** Every card's note it starts with (section S14), by card id. None by default. */
  notes?: Readonly<Record<string, Note>>;
  /** The person it starts with (sections S2a, S32, S31a). The golden profile with no avatar by default. */
  profile?: Profile;
  /** The person's preferences it starts with. The defaults by default: nothing saved. */
  preferences?: Preferences;
  /** The person's onboarding and tour progress it starts with. Both pending by default. */
  progress?: Progress;
  /** The saved views it starts with, across projects (section S6a). None by default. */
  savedViews?: readonly SavedView[];
  /**
   * The provider rows it starts with (section S28). The golden list by default: Anthropic saved,
   * DeepSeek empty, OpenRouter invalid, Ollama saved with its address.
   */
  providers?: readonly Provider[];
  /** The checks a provider's connection test answers with. A passing test by default. */
  providerChecks?: (row: Provider) => TestCheck[];
  /** Refuses a key with its own sentence, the way the daemon's own value check does. */
  providerKeyRefused?: (row: Provider, key: string) => string | undefined;
  /**
   * The connection rows it starts with (section S29a). Every connection the daemon knows, none set
   * up, by default.
   */
  integrations?: readonly Integration[];
  /** The checks a connection's test answers with. A passing GitHub test by default. */
  integrationChecks?: (row: Integration) => TestCheck[];
  /** Refuses a pasted GitHub token with its own sentence, the way GitHub's own check does. */
  integrationSaveRefused?: (id: string, body: Record<string, unknown>) => string | undefined;
  /** The cost and awake ceilings it starts with (sections S19b and S26b). The golden list by default. */
  limits?: readonly Limit[];
  /** The role templates it starts with (section S27). The golden list by default, no project overridden. */
  roles?: readonly Role[];
  /** The standing notices it starts with (section S23). The golden `notice-list` by default. */
  notices?: readonly Notice[];
  /** The sleep settings it starts with (section S26a). The golden `sleep-settings` by default. */
  sleep?: SleepSettings;
  /** The schedules it starts with (section S30). The golden `schedule-list` by default. */
  schedules?: readonly Schedule[];
  /** The alert settings it starts with (section S26c). The golden `alert-settings` by default. */
  alerts?: AlertSettings;
  /** The projects that have CI data (section S21). None by default, so each reads as not connected. */
  ci?: readonly ProjectCI[];
  /** A card's preview it starts with (section S13), one per card. None by default: all stopped. */
  previews?: readonly Preview[];
  /** The Integrator state it starts with, one per project (the Integration view). None: all idle. */
  mergeFlows?: readonly IntegrationState[];
  /** Whether Marshal can find a browser to shoot with. True by default, so a screenshot is taken. */
  previewBrowser?: boolean;
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
  /** Every card's restore points it holds now, by card id, newest first (B5.3). */
  checkpoints: Record<string, Checkpoint[]>;
  /** The person it holds now: the profile, the progress, the preferences, and every saved view. */
  me: MeStore;
  /** The provider rows and their stored values it holds now (section S28). */
  providers: ProviderStore;
  /** The connection rows it holds now and their stored tests (section S29a, B6.1 and B6.7). */
  integrations: IntegrationStore;
  /** The cost and awake ceilings it holds now (sections S19b and S26b). */
  limits: LimitsStore;
  /** The role templates and the projects that keep their own version of them it holds now (S27). */
  roles: RolesStore;
  /** The notices it holds now, and the stream a change is announced on (section S23). */
  notices: NoticesStore;
  /** The sleep settings it holds now (section S26a). */
  sleep: SleepStore;
  /** The schedules it holds now (section S30). */
  schedules: ScheduleStore;
  /** The alert settings it holds now (section S26c). */
  alerts: AlertsStore;
  /** Every card's checks, checklists, comments, and members it holds now (sections S12, S15, S16). */
  cardPanel: CardPanelStore;
  /** Every project's CI health it holds now (section S21). A test seeds it and reads it back. */
  ci: CIStore;
  /** Every card's preview it holds now (section S13), by the daemon's own card id. */
  previews: PreviewStore;
  /** Every project's merge queue and delivered cards it holds now, and the folders opened. */
  mergeFlow: MergeFlowStore;
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
  // An empty string clears the choice, so the default branch stands in for it again.
  if (typeof body.integrationBranch === "string") {
    project.integrationBranch = body.integrationBranch.trim() || project.defaultBranch;
  }
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
  providers: ProviderStore;
  integrations: IntegrationStore;
  limits: LimitsStore;
  roles: RolesStore;
  notices: NoticesStore;
  sleep: SleepStore;
  schedules: ScheduleStore;
  alerts: AlertsStore;
  cardPanel: CardPanelStore;
  ci: CIStore;
  previews: PreviewStore;
  mergeFlow: MergeFlowStore;
}

/**
 * The paired devices (B9.1, B9.2, section S2b) and the tailnet status (B9.1): the four routes the
 * profile draws. The fake daemon answers with one phone, so a component test has a real list to
 * read; a revoke marks the row rather than dropping it, because the screen says a device was
 * removed instead of watching it vanish, and the status answers "off", which is what a daemon that
 * was never started with `--tailnet` truthfully is.
 */
/** The one pairing code the fake daemon makes and accepts. */
export const FAKE_PAIRING_CODE = "7QX-2LD";

const FAKE_DEVICES = [
  {
    id: "01H1234567890ABCDEFGHJKMNPQ",
    name: "Pixel 8",
    kind: "mobile",
    pairedAt: "2026-09-20T10:00:00.000Z",
    lastSeenAt: "2026-09-28T09:00:00.000Z",
    revoked: false,
  },
  {
    id: "01H1234567890ABCDEFGHJKMNPR",
    name: "iPad Air",
    kind: "mobile",
    pairedAt: "2026-09-22T12:00:00.000Z",
    lastSeenAt: null,
    revoked: false,
  },
];

function answerDeviceRoute(router: Router, request: FakeRequest): Response | null {
  const key = keyOf(request);
  if (key === "GET /v1/me/devices") {
    return jsonAnswer({ devices: FAKE_DEVICES, serverTime: router.now() });
  }
  if (key === "POST /v1/me/devices/pairing-code") {
    return jsonAnswer({
      code: FAKE_PAIRING_CODE,
      expiresAt: "2026-09-28T10:00:00.000Z",
      serverTime: router.now(),
    });
  }
  if (request.method === "DELETE" && pathOf(request.url).startsWith("/v1/me/devices/")) {
    return new Response(null, { status: 204 });
  }
  if (key === "GET /v1/tailnet") {
    return jsonAnswer({
      enabled: false,
      state: "off",
      hostname: "",
      dnsName: "",
      ips: [],
      identity: "",
      loginUrl: "",
      funnel: false,
      error: "",
      serverTime: router.now(),
    });
  }
  return null;
}

/**
 * The route that trades a pairing code for a token, which takes no token of its own. The one code
 * the fake makes is accepted and answers the daemon's own token, so a device that pairs is signed in;
 * any other is refused with the daemon's own 401, the same for a wrong, spent, or expired code.
 */
function answerPairRoute(router: Router, request: FakeRequest): Response {
  const body = bodyOf(request);
  if (body.code !== FAKE_PAIRING_CODE) {
    return refuse(
      STATUS.unauthorized,
      "unauthorized",
      "Sign in again. This device's token is missing or no longer valid.",
    );
  }
  const device = {
    ...FAKE_DEVICES[0],
    name: String(body.name ?? ""),
    kind: String(body.kind ?? ""),
  };
  return jsonAnswer({ token: router.token(), device, serverTime: router.now() });
}

/** The alert settings and a card's panel, which are separate stores answered one after the other. */
function answerAlertsOrPanel(router: Router, request: FakeRequest): Response | undefined {
  return (
    answerAlertsRoute(router.alerts, request) ?? answerCardPanelRoute(router.cardPanel, request)
  );
}

/** The folders of a small pretend disk: the home folder, a code folder with one repository, and a notes folder. */
const FAKE_HOME = "/Users/ada";
const FAKE_DISK: Record<string, { name: string; repo?: boolean }[]> = {
  [FAKE_HOME]: [{ name: "code" }, { name: "notes" }],
  [`${FAKE_HOME}/code`]: [{ name: "marshal", repo: true }, { name: "scratch" }],
  [`${FAKE_HOME}/code/marshal`]: [{ name: "apps" }],
};

function answerFolders(router: Router, request: FakeRequest): Response {
  const asked = new URL(request.url, "http://fake-daemon").searchParams.get("path") ?? "";
  const path = asked === "" || asked === "~" ? FAKE_HOME : asked;
  const inside = FAKE_DISK[path];
  if (!inside && !Object.keys(FAKE_DISK).some((known) => known.startsWith(`${path}/`))) {
    return refuse(
      STATUS.notFound,
      "not_found",
      "Marshal cannot find that folder. It may have been removed.",
    );
  }
  const parent = path === "/" ? "" : path.slice(0, path.lastIndexOf("/")) || "/";
  return jsonAnswer({
    path,
    parent,
    home: FAKE_HOME,
    isGitRepo: path === `${FAKE_HOME}/code/marshal`,
    folders: (inside ?? []).map((entry) => ({
      name: entry.name,
      path: `${path}/${entry.name}`,
      isGitRepo: entry.repo === true,
    })),
    truncated: false,
    serverTime: router.now(),
  });
}

/** The agent catalog, a scan of it, and the test of one agent. */
function answerAgentRoute(router: Router, request: FakeRequest, key: string): Response | undefined {
  if (request.method === "GET" && pathOf(request.url) === "/v1/folders") {
    return answerFolders(router, request);
  }
  const tested = /^\/v1\/agents\/([^/]+)\/test$/.exec(pathOf(request.url))?.[1];
  if (request.method === "POST" && tested) return answerAgentTest(router, tested);
  if (key === "GET /v1/agents" || key === "POST /v1/agents/refresh") {
    return jsonAnswer({ ...router.catalog, serverTime: router.now() });
  }
  return undefined;
}

/** A test of one agent: a pass for an agent or tool the catalog lists, and not found for any other. */
function answerAgentTest(router: Router, id: string): Response {
  const known =
    router.catalog.agents.some((agent) => agent.kind === id) ||
    router.catalog.tools.some((tool) => tool.id === id);
  if (!known)
    return refuse(
      STATUS.notFound,
      "not_found",
      "Marshal cannot find that agent. It may have been removed.",
    );
  return jsonAnswer({
    connectionId: id,
    ok: true,
    checks: [{ name: "Installed", state: "passed", message: "It is on this computer." }],
    ranAt: router.now(),
  });
}

/** The routes of the settings screens and a card's panel, tried in turn until one answers. */
function answerSettingsRoutes(
  router: Router,
  request: FakeRequest,
  projectExists: (pid: string) => boolean,
): Response | undefined {
  return (
    answerProviderRoute(router.providers, request) ??
    answerIntegrationRoute(router.integrations, request) ??
    answerLimitRoute(router.limits, request) ??
    answerRoleRoute(router.roles, request, projectExists) ??
    answerNoticeRoute(router.notices, request) ??
    answerSleepRoute(router.sleep, request) ??
    answerAlertsOrPanel(router, request)
  );
}

/** Answers one request the way the real daemon's router does, including who may ask. */
function answer(router: Router, request: FakeRequest): Response {
  const key = keyOf(request);
  // The dev server's own route, which the dev build asks for the token.
  if (key === "GET /__marshal/dev-token") return jsonAnswer({ token: FAKE_TOKEN });
  if (key === "GET /v1/health") {
    return jsonAnswer({ ...golden<Health>("health"), serverTime: router.now() });
  }
  if (key === "POST /v1/devices/pair") return answerPairRoute(router, request);
  if (request.headers.authorization !== `Bearer ${router.token()}`) {
    return refuse(
      STATUS.unauthorized,
      "unauthorized",
      "Sign in again. This device's token is missing or no longer valid.",
    );
  }
  const projectExists = (pid: string) => router.projects.some((p) => p.id === pid);
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
  const agentAnswer = answerAgentRoute(router, request, key);
  if (agentAnswer) return agentAnswer;
  const settings = answerSettingsRoutes(router, request, projectExists);
  if (settings) return settings;
  const schedule = answerScheduleRoute(router.schedules, request);
  if (schedule) return schedule;
  const calendar = answerCalendarRoute(router.schedules, request);
  if (calendar) return calendar;
  // Every project's CI health in one answer (section S21), which is what the boards' and Home's
  // lists read: the route is the top-level `/v1/ci` and belongs to no project.
  const ci = answerCIRoute(router.ci, request);
  if (ci) return ci;
  // A project's merge queue and a card's retry, undo, and worktree (the Integration view).
  const mergeFlow = answerMergeFlowRoute(router.mergeFlow, request);
  if (mergeFlow) return mergeFlow;
  const cards = answerCardRoute(
    router.cards,
    request,
    (pid) => router.projects.some((p) => p.id === pid),
    (pid) => router.projects.find((p) => p.id === pid)?.name ?? pid,
    (pid) => router.projects.find((p) => p.id === pid)?.bypassLocked ?? false,
  );
  if (cards) return cards;
  const chat = answerChatRoute(router.chats, request, (pid) =>
    router.projects.some((p) => p.id === pid),
  );
  if (chat) return chat;
  const device = answerDeviceRoute(router, request);
  if (device) return device;
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

/** The store-shaped pieces of the daemon, each holding one slice of it and answering its own routes. */
interface Slices {
  me: MeStore;
  providers: ProviderStore;
  integrations: IntegrationStore;
  limits: LimitsStore;
  roles: RolesStore;
  notices: NoticesStore;
  sleep: SleepStore;
  schedules: ScheduleStore;
  alerts: AlertsStore;
  cardPanel: CardPanelStore;
  ci: CIStore;
  previews: PreviewStore;
  mergeFlow: MergeFlowStore;
}

/**
 * Builds every store the daemon holds: the person (the profile, the progress, the preferences, and
 * the saved views), the provider rows and their stored values, the connection rows and their stored
 * tests, the cost and awake ceilings, the role templates, the standing notices, the sleep settings,
 * every project's CI health, and every card's live preview. It is apart from `createFakeDaemon` so
 * that function stays a wiring list rather than a wall of literals.
 */
function createSlices(
  options: FakeDaemonOptions,
  projects: readonly Project[],
  cards: Card[],
  emit: Publish,
  now: () => string,
): Slices {
  const profile = structuredClone(options.profile ?? wireProfile());
  // The command a card's preview would run is its project's own, so the tab can say what pressing
  // Start does; a project that has none is refused by the start route, as the daemon refuses it.
  const commandOf = (card: Card): string =>
    projects.find((project) => project.id === card.projectId)?.devCommand ?? "";
  return {
    me: {
      profile,
      progress: structuredClone(options.progress ?? pendingProgress()),
      preferences: structuredClone(options.preferences ?? emptyPreferences()),
      savedViews: structuredClone([...(options.savedViews ?? [])]),
      // A profile that starts with an avatar has one to serve.
      avatar: profile.avatarUrl ? { type: "image/png", size: 70 } : null,
      dev: options.dev ?? false,
      publish: emit,
      now,
    },
    providers: createProviderStore({
      providers: options.providers,
      checks: options.providerChecks,
      refuseKey: options.providerKeyRefused,
      nowMs: () => Date.now() + (options.clockSkewMs ?? 0),
    }),
    integrations: createIntegrationStore({
      integrations: options.integrations,
      checks: options.integrationChecks,
      refuseSave: options.integrationSaveRefused,
      nowMs: () => Date.now() + (options.clockSkewMs ?? 0),
    }),
    limits: createLimitsStore({ limits: options.limits }),
    roles: createRolesStore({ roles: options.roles, now }),
    notices: createNoticesStore({ notices: options.notices, publish: emit, now }),
    sleep: createSleepStore({ settings: options.sleep }),
    schedules: createScheduleStore({ schedules: options.schedules, now }),
    alerts: createAlertsStore({ settings: options.alerts }),
    cardPanel: createCardPanelStore({ publish: emit, now }),
    ci: createCIStore({ projects: options.ci, now }),
    previews: createPreviewStore({
      previews: options.previews,
      commandOf,
      hasBrowser: options.previewBrowser ?? true,
      publish: emit,
      now,
    }),
    mergeFlow: createMergeFlowStore({
      states: options.mergeFlows,
      cards: () => cards,
      hasProject: (projectId) => projects.some((project) => project.id === projectId),
      publish: emit,
      now,
    }),
  };
}

/**
 * Every fixture `createFakeDaemon` holds, each its own copy: a test that changes a project, a
 * card, or anything else through the daemon must never change the shared fixture the options
 * came from. Split out of `createFakeDaemon` itself only to keep that function under the lines
 * limit; nothing here depends on the router, the sockets, or the clock.
 */
function cloneFixtures(options: FakeDaemonOptions) {
  // A card's restore points (B5.3), by card id, newest first, each list copied so a test's
  // fixture is never changed by what the routes answer.
  const checkpoints: Record<string, Checkpoint[]> = {};
  for (const [id, list] of Object.entries(options.checkpoints ?? {})) {
    checkpoints[id] = structuredClone([...list]);
  }
  // A card's note (section S14), by card id, each copied for the same reason the checkpoints are.
  const notes: Record<string, Note> = {};
  for (const [id, note] of Object.entries(options.notes ?? {})) {
    notes[id] = structuredClone(note);
  }
  return {
    projects: structuredClone([...(options.projects ?? [])]),
    catalog: options.catalog ?? golden<AgentCatalog>("agents"),
    cards: structuredClone([...(options.cards ?? [])]),
    history: structuredClone([...(options.history ?? [])]),
    labels: structuredClone([...(options.labels ?? [])]),
    chats: structuredClone([...(options.chats ?? [])]),
    chatMessages: structuredClone([...(options.chatMessages ?? [])]),
    activity: structuredClone([...(options.activity ?? [])]),
    diffs: structuredClone(options.diffs ?? {}),
    checkpoints,
    notes,
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
  const {
    projects,
    catalog,
    cards,
    history,
    labels,
    chats,
    chatMessages,
    activity,
    diffs,
    checkpoints,
    notes,
  } = cloneFixtures(options);

  const emit = publisher(sockets, state, now);
  const terminals = createTerminalRouter({ publish: emit, seqNow: () => state.seq });
  termRef.current = terminals;
  const slices = createSlices(options, projects, cards, emit, now);
  const router: Router = {
    token: () => state.token,
    now,
    projects,
    state: { projects, publish: emit, now },
    catalog,
    cards: {
      cards,
      labels,
      history,
      activity,
      diffs,
      checkpoints,
      notes,
      previews: slices.previews,
      publish: emit,
      now,
    },
    chats: { chats, messages: chatMessages, answer: options.chatAnswer, publish: emit, now },
    ...slices,
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
    checkpoints,
    me: slices.me,
    providers: slices.providers,
    integrations: slices.integrations,
    limits: slices.limits,
    roles: slices.roles,
    notices: slices.notices,
    sleep: slices.sleep,
    schedules: slices.schedules,
    alerts: slices.alerts,
    cardPanel: slices.cardPanel,
    ci: slices.ci,
    previews: slices.previews,
    mergeFlow: slices.mergeFlow,
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
