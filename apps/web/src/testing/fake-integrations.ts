/**
 * The connection routes of the fake daemon (docs/backend-checklist.md B6.1, B6.7, B7.4; sections
 * S29a and S29b). Every route answers the way the daemon's own handler does: the list carries every
 * connection Marshal knows, whether or not it is set up; a save stores the settings (GitHub's is a
 * pasted token), tests the connection as part of the same call, and answers the whole list; a test
 * asked for inside the cooldown is refused with how long to wait; and nothing secret ever comes back.
 *
 * GitHub's sign-in (`/v1/integrations/github/connect`) is a small machine: a start answers a
 * pending code, and each read answers the next entry of `github.script`, or the flow as it stands,
 * or the stored connection. A test moves a sign-in on by queueing the answers it wants to read.
 *
 * The connections of phases that have not reached this file yet are listed and read as not
 * connected, exactly as the daemon's own `known()` does, so a cutover does not renumber anything.
 * Obsidian is the one exception: it is Marshal's own connection and not a person's, so it starts
 * connected the way `integrations.go`'s `vaultStatus` always answers when a vault is set (`knownRows`
 * below). Nothing here reaches GitHub or a real vault: the test's checks are made up, because what
 * the frontend needs from these routes is the shape of the answer.
 *
 * A row's status and its one sentence are not stored, they are derived from the last test the way
 * `integrations.go`'s `rowToWire` derives them: the failed check's fix, the passing test's Summary
 * check, or the daemon's own "run the test" when nothing has tested it yet. A row is "saved" here
 * when its status is not `none`, which is what the daemon stores a config and a secret for - except
 * Obsidian, which has nothing to save and is never `none` in the first place.
 */
import type {
  DetectTelegramChatAnswer,
  GitHubConnect,
  GitHubInstallation,
  Integration,
  IntegrationStatus,
  TestCheck,
  TestResult,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";

const STATUS = { ok: 200, badRequest: 400, notFound: 404, conflict: 409, unavailable: 503 };

/** The daemon's cooldown between two tests of one connection (connectiontest.DefaultCooldown). */
const DEFAULT_COOLDOWN_MS = 5000;
const MS_PER_SECOND = 1000;
const GITHUB = "github";
const OBSIDIAN = "obsidian";
const NTFY = "ntfy";
const GDRIVE = "gdrive";
const BAD_REQUEST = 400;
const SAVE_PATH = /^\/v1\/integrations\/([^/]+)$/;
const CONNECT_PATH = "/v1/integrations/github/connect";
const TOKEN_PATH = "/v1/integrations/github/token";
const TOKEN_TEST_PATH = "/v1/integrations/github/token/test";
const VERIFICATION_URI = "https://github.com/login/device";
const INSTALL_URL = "https://github.com/apps/marshal-kanban/installations/new";
const CODE_LIFETIME_MS = 900_000;
const TEST_PATH = /^\/v1\/integrations\/([^/]+)\/test$/;
const LIST_PATH = "/v1/integrations";
/**
 * The check whose message is a saved row's own sentence, the sentence a passing GitHub test writes
 * into it, and the sentence such a row shows before anything has tested it. All three are the
 * daemon's own (`integrations.go`'s `summaryOf` and `detailFor`, `test.go`'s `summaryCheck`), and
 * the answers below are built the way those files build them.
 */
const SUMMARY = "Summary";
const PASSED_DETAIL = "The GitHub App works and Marshal can use it.";
const UNTESTED_DETAIL = "Connected. Run the test to check it.";
/**
 * Obsidian's own sentence, the way `integrations.go`'s `vaultStatus` always answers it once a vault
 * is set: unlike a person's own connection, this is not replaced by a passing test's Summary message
 * or by "run the test" before one has run - only a *failed* test's own detail ever replaces it
 * (`rowToWire`'s self-owned branch keeps `vaultStatus()`'s answer except when `tested && !last.OK`).
 */
const OBSIDIAN_DETAIL = "Vault at ~/fake/vault.";

/** The four Google file connections: the name each one is shown by, and what its test is filed under. */
export const GOOGLE_FILE_SERVICES: Readonly<Record<string, { name: string; kind: string }>> = {
  gdrive: { name: "Google Drive", kind: "drive" },
  gdocs: { name: "Google Docs", kind: "docs" },
  gsheets: { name: "Google Sheets", kind: "sheets" },
  gslides: { name: "Google Slides", kind: "slides" },
};

/** The folder Marshal puts the files it makes in, until Google Drive's own setting says another. */
export const DEFAULT_DRIVE_FOLDER = "Marshal";
const FOLDER_NAME = /^[^\p{Cc}]{1,100}$/u;

/**
 * Every connection the daemon knows, in the order the settings screen shows them, with the kind each
 * one's test is filed under. It mirrors `daemon/internal/integrations/integrations.go`'s `known()`.
 */
const KNOWN: readonly { id: string; kind: string }[] = [
  { id: GITHUB, kind: "github" },
  { id: "trello", kind: "trello" },
  { id: "gcal", kind: "calendar" },
  { id: "gmail", kind: "gmail" },
  ...Object.entries(GOOGLE_FILE_SERVICES).map(([id, { kind }]) => ({ id, kind })),
  { id: "telegram", kind: "telegram" },
  { id: "discord", kind: "discord" },
  { id: "ntfy", kind: "ntfy" },
  { id: "obsidian", kind: "obsidian" },
];

/** GitHub's sign-in and the connection it makes, as the fake daemon holds them. */
interface GitHubFake {
  /** The sign-in under way, or the one that just ended, or null when there is none. */
  flow: GitHubConnect | null;
  /** What the next reads answer, one entry each, before they answer the flow or the stored connection. */
  script: GitHubConnect[];
  /** How the stored connection was made. */
  mode: "oauth" | "token";
  /** The GitHub user the stored connection is for. */
  login: string;
  /** The accounts the App is installed on, for a connection made by signing in. */
  installations: GitHubInstallation[];
  /** The code a started sign-in is given. */
  userCode: string;
  /** When set, starting a sign-in is refused with this sentence, as when GitHub cannot be reached. */
  startRefusal?: string;
}

/** The connections and what is stored for them, as the fake daemon holds them. */
export interface IntegrationStore {
  /** GitHub's sign-in, and how its connection was made. */
  github: GitHubFake;
  /**
   * The connections it knows, in the daemon's order. A row whose status is not `none` has a config
   * and a secret stored, and its sentence is what it shows until a test replaces it.
   */
  rows: Integration[];
  /** The name of the Drive folder Marshal puts the files it makes in. */
  driveFolder: string;
  /** Connections with a saved setting and no access from Google yet: the row says to grant it. */
  ungranted: string[];
  /** When each connection was last tested, in ms. No entry means it was never tested. */
  testedAt: Record<string, number>;
  /** The last test's own result, by connection id. */
  lastTest: Record<string, TestResult>;
  /** What finding a Telegram bot's chat answers. A found private chat by default. */
  detectedChat: DetectTelegramChatAnswer;
  /** When set, finding a chat is refused with this sentence, as the daemon refuses a token Telegram does not accept. */
  detectRefusal?: string;
  /** Refuses a save with its own sentence, or answers undefined to store it. */
  refuseSave: (id: string, body: Record<string, unknown>) => string | undefined;
  /** The checks one connection's test answers with. */
  checks: (row: Integration) => TestCheck[];
  /** How long a connection waits between tests. */
  cooldownMs: number;
  /** The daemon's clock, in ms. */
  nowMs: () => number;
}

export interface FakeIntegrationOptions {
  /** The rows it starts with. Every known connection, none set up, by default. */
  integrations?: readonly Integration[];
  /** What finding a Telegram bot's chat answers. A found private chat by default. */
  detectedChat?: DetectTelegramChatAnswer;
  /** Refuses a save with its own sentence, as a test may need. GitHub's is a token save. */
  refuseSave?: (id: string, body: Record<string, unknown>) => string | undefined;
  /** The GitHub user a connection is for. "ada" by default. */
  githubLogin?: string;
  /** The accounts the App is installed on once a sign-in connects. Ada's, all repositories, by default. */
  githubInstallations?: readonly GitHubInstallation[];
  /** The checks a test answers with. A passing test by default, GitHub's or Obsidian's own shape. */
  checks?: (row: Integration) => TestCheck[];
  /** How long a connection waits between tests, in ms. The daemon's 5s by default. */
  cooldownMs?: number;
  /** Its clock, in ms. `Date.now` by default. */
  nowMs?: () => number;
}

/**
 * The rows the daemon lists before anything is set up: one per known connection, none connected -
 * except Obsidian, which nobody sets up (section S29b, docs/architecture.md section 18). It is
 * connected the moment Marshal has a vault, which the real daemon always does, so this fake starts
 * it connected too, exactly as `integrations.go`'s `vaultStatus` would answer for a vault that is
 * simply not tested yet.
 */
function knownRows(): Integration[] {
  return KNOWN.map(({ id, kind }) =>
    id === OBSIDIAN
      ? { id, kind, st: "connected" as const, detail: OBSIDIAN_DETAIL }
      : { id, kind, st: "none" as const, detail: "" },
  );
}

export function createIntegrationStore(options: FakeIntegrationOptions = {}): IntegrationStore {
  return {
    github: {
      flow: null,
      script: [],
      mode: "oauth",
      login: options.githubLogin ?? "ada",
      installations: structuredClone([
        ...(options.githubInstallations ?? [
          { account: "ada", kind: "user", allRepositories: true },
        ]),
      ]),
      userCode: "WDJB-MJHT",
    },
    rows: structuredClone([...(options.integrations ?? knownRows())]),
    driveFolder: DEFAULT_DRIVE_FOLDER,
    ungranted: [],
    testedAt: {},
    lastTest: {},
    detectedChat: options.detectedChat ?? {
      found: true,
      chatId: "777",
      name: "Ada Okafor",
      kind: "private",
      message: "",
    },
    refuseSave: options.refuseSave ?? defaultRefusal,
    checks: options.checks ?? defaultChecksFor,
    cooldownMs: options.cooldownMs ?? DEFAULT_COOLDOWN_MS,
    nowMs: options.nowMs ?? Date.now,
  };
}

/** Answers one connection route, or null when the request is not one. */
export function answerIntegrationRoute(
  store: IntegrationStore,
  request: FakeRequest,
): Response | null {
  const path = request.url.replace(/^https?:\/\/[^/]+/, "");
  if (path === LIST_PATH && request.method === "GET") return listAnswer(store);
  if (path === "/v1/integrations/telegram/detect-chat" && request.method === "POST") {
    return store.detectRefusal
      ? errorAnswer(BAD_REQUEST, "invalid_argument", store.detectRefusal)
      : jsonAnswer(store.detectedChat);
  }
  const github = answerGitHubRoute(store, path, request);
  if (github) return github;
  const test = TEST_PATH.exec(path);
  if (test && request.method === "POST") {
    return testConnection(store, decodeURIComponent(test[1] ?? ""));
  }
  const one = SAVE_PATH.exec(path);
  if (!one) return null;
  const id = decodeURIComponent(one[1] ?? "");
  if (request.method === "PUT") return saveConnection(store, id, request);
  if (request.method === "DELETE") return removeConnection(store, id);
  return null;
}

function find(store: IntegrationStore, id: string): Integration | undefined {
  return store.rows.find((row) => row.id === id);
}

const notFound = (): Response =>
  errorAnswer(STATUS.notFound, "not_found", "Marshal has no connection with that id.");

function conflict(message: string, retryAfterMs: number): Response {
  return jsonAnswer(
    { error: { code: "conflict", message, details: { retryAfterMs: String(retryAfterMs) } } },
    STATUS.conflict,
  );
}

function bodyOf(request: FakeRequest): Record<string, unknown> {
  try {
    return request.body ? (JSON.parse(request.body) as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

function listAnswer(store: IntegrationStore): Response {
  return jsonAnswer({
    integrations: store.rows.map((row) => wireRow(store, row)),
    serverTime: new Date(store.nowMs()).toISOString(),
  });
}

/**
 * One connection as the daemon answers it: no config is "not connected" with nothing else, and a
 * saved one carries what its last test decided (`integrations.go`'s `rowToWire`).
 */
function wireRow(store: IntegrationStore, row: Integration): Integration {
  if (row.st === "none") {
    const name = GOOGLE_FILE_SERVICES[row.id]?.name;
    const detail = name && store.ungranted.includes(row.id) ? grantDetail(name) : "";
    return { id: row.id, kind: row.kind, st: "none", detail };
  }
  const last = store.lastTest[row.id];
  return {
    id: row.id,
    kind: row.kind,
    st: statusFor(last),
    detail: detailFor(row, last),
    ...(last ? { lastTest: last } : {}),
  };
}

/** A saved connection's status: a test that failed any check needs attention; anything else is fine. */
function statusFor(last: TestResult | undefined): IntegrationStatus {
  return last && !last.ok ? "error" : "connected";
}

/**
 * The one sentence under a row. Obsidian keeps its own vault sentence whatever a test finds, except
 * a failure - the self-owned rule `rowToWire` follows, and the reason this takes `row` rather than
 * just its id: a self-owned row's "connected" detail is never derived from a test the way a
 * person's own connection's is. Everyone else: the failed check's fix, the passing test's summary,
 * or the reminder that nothing has tested it yet.
 */
function detailFor(row: Integration, last: TestResult | undefined): string {
  const failed = last?.checks.find((check) => check.state === "failed");
  if (row.id === OBSIDIAN) return failed ? (failed.fix ?? failed.message) : row.detail;
  if (!last) return row.detail || UNTESTED_DETAIL;
  if (failed) return failed.fix ?? failed.message;
  const summary = last.checks.find((check) => check.name === SUMMARY);
  return summary ? summary.message : "Connected.";
}

/** What a Google connection says while its settings are saved and Google has not been asked yet. */
const grantDetail = (name: string): string => `Grant access to finish connecting ${name}.`;

/** The Drive folder's name: one to a hundred characters, none of them a control character. */
function folderRefusal(folder: unknown): string | undefined {
  return typeof folder === "string" && FOLDER_NAME.test(folder)
    ? undefined
    : "The folder name must be 1 to 100 characters, with no control characters.";
}

/** Google Drive's one setting, the folder. It is saved whether or not Google was asked yet. */
function saveDrive(store: IntegrationStore, request: FakeRequest): Response {
  const { folder } = bodyOf(request);
  const refusal = folderRefusal(folder);
  if (refusal) return errorAnswer(STATUS.badRequest, "invalid_argument", refusal);
  store.driveFolder = String(folder);
  const row = find(store, GDRIVE);
  if (row?.st === "none" && !store.ungranted.includes(GDRIVE)) store.ungranted.push(GDRIVE);
  return listAnswer(store);
}

function saveConnection(store: IntegrationStore, id: string, request: FakeRequest): Response {
  // Only ntfy and Google Drive have a generic save shape here (GitHub's is the token route), and an
  // id with none is refused rather than silently accepted, as the daemon's `saveIntegration` does.
  if (id === GDRIVE) return saveDrive(store, request);
  if (id !== NTFY) return notFound();
  const row = find(store, id);
  if (!row) return notFound();
  const refusal = ntfyRefusal(bodyOf(request));
  if (refusal) return errorAnswer(STATUS.badRequest, "invalid_argument", refusal);
  row.st = "connected";
  // The daemon tests a connection that has just changed, and does so without the cooldown
  // (connectiontest.RunAfterConnect), so the save's own answer already carries the test.
  runTest(store, row, store.nowMs());
  return listAnswer(store);
}

function removeConnection(store: IntegrationStore, id: string): Response {
  const row = find(store, id);
  if (!row) return notFound();
  delete store.testedAt[id];
  delete store.lastTest[id];
  store.ungranted = store.ungranted.filter((entry) => entry !== id);
  if (id === GITHUB) store.github.flow = null;
  row.st = "none";
  row.detail = "";
  return listAnswer(store);
}

function testConnection(store: IntegrationStore, id: string): Response {
  const row = find(store, id);
  if (!row) return notFound();
  const at = store.nowMs();
  const wait = cooldownLeft(store, id, at);
  if (wait > 0) {
    const seconds = Math.ceil(wait / MS_PER_SECOND);
    return conflict(
      `This connection was tested a moment ago. Try again in ${seconds} seconds.`,
      wait,
    );
  }
  return jsonAnswer(runTest(store, row, at));
}

function cooldownLeft(store: IntegrationStore, id: string, at: number): number {
  const last = store.testedAt[id];
  if (last === undefined) return 0;
  const elapsed = at - last;
  return elapsed >= store.cooldownMs ? 0 : store.cooldownMs - elapsed;
}

/**
 * Runs one test and saves its result. The row's status and its sentence are left alone: both are
 * derived from this result when the list is answered, as the daemon derives them.
 */
function runTest(store: IntegrationStore, row: Integration, at: number): TestResult {
  const checks = store.checks(row);
  const result: TestResult = {
    connectionId: row.id,
    checks,
    ok: !checks.some((check) => check.state === "failed"),
    ranAt: new Date(at).toISOString(),
  };
  store.testedAt[row.id] = at;
  store.lastTest[row.id] = result;
  return result;
}

/** The GitHub connection test's own checks, as `daemon/internal/integrations/test.go` asks them. */
function githubChecks(_row: Integration): TestCheck[] {
  return [
    // The one check whose message is the row's own sentence, as every connection test names one.
    { name: SUMMARY, state: "passed", message: PASSED_DETAIL },
    { name: "App", state: "passed", message: "The GitHub App is installed." },
    { name: "Repositories", state: "passed", message: "3 repositories are visible." },
    {
      name: "Permissions",
      state: "passed",
      message: "Issues, pull requests, and Actions are allowed.",
    },
    { name: "Webhook", state: "passed", message: "A ping reached Marshal." },
  ];
}

/** The Obsidian vault test's own checks, as `daemon/internal/integrations/obsidian.go` asks them. */
function obsidianChecks(_row: Integration): TestCheck[] {
  return [
    { name: SUMMARY, state: "passed", message: "Marshal's vault is ready to open in Obsidian." },
    { name: "Vault folder", state: "passed", message: "The vault folder is there." },
    {
      name: "Vault writable",
      state: "passed",
      message: "The vault folder's permissions let Marshal write in it.",
    },
  ];
}

/** The Google file connections' own checks: access, Drive, and the one API each is for. */
function googleFileChecks(row: Integration): TestCheck[] {
  const name = GOOGLE_FILE_SERVICES[row.id]?.name ?? "";
  const folder = `“${DEFAULT_DRIVE_FOLDER}”`;
  const summary =
    row.id === GDRIVE
      ? `${name} works. Files go in the folder ${folder}.`
      : `${name} works. Marshal makes documents in the folder ${folder} and reads any you share by link.`;
  return [
    { name: SUMMARY, state: "passed", message: summary },
    { name: "Access", state: "passed", message: "Google accepted Marshal's access." },
    { name: "Drive", state: "passed", message: "Google Drive answered." },
    row.id === GDRIVE
      ? { name: "Folder", state: "passed", message: `The folder ${folder} is there.` }
      : { name: name.replace("Google ", ""), state: "passed", message: `${name} answered.` },
  ];
}

/** Which connection's checks to run, by id, so testing one connection never answers another's shape. */
function defaultChecksFor(row: Integration): TestCheck[] {
  if (row.id === NTFY) return ntfyChecks();
  if (GOOGLE_FILE_SERVICES[row.id]) return googleFileChecks(row);
  return row.id === OBSIDIAN ? obsidianChecks(row) : githubChecks(row);
}

/** ntfy needs only a topic: an open topic on a public server has no token. */
function ntfyRefusal(body: Record<string, unknown>): string | undefined {
  const topic = typeof body.topic === "string" ? body.topic.trim() : "";
  return topic ? undefined : "Choose the ntfy topic Marshal should send notices to.";
}

/** The ntfy test's own checks, as `daemon/internal/chatbot/ntfy.go` asks them. */
function ntfyChecks(): TestCheck[] {
  const message = "Marshal sent a test message to the topic.";
  return [
    {
      name: SUMMARY,
      state: "passed",
      message: "ntfy is set up and Marshal can send notices to it.",
    },
    { name: "Topic", state: "passed", message },
  ];
}

/** A token must be there: an empty one is what GitHub refuses first. */
function defaultRefusal(_id: string, body: Record<string, unknown>): string | undefined {
  const token = typeof body.token === "string" ? body.token.trim() : "";
  return token === "" ? "GitHub did not accept that token." : undefined;
}

/** GitHub's sign-in and token routes. Null when the request is not one of them. */
function answerGitHubRoute(
  store: IntegrationStore,
  path: string,
  request: FakeRequest,
): Response | null {
  if (path === CONNECT_PATH) {
    if (request.method === "POST") return startConnect(store);
    if (request.method === "GET") return readConnect(store);
    if (request.method === "DELETE") return cancelConnect(store);
  }
  if (path === TOKEN_PATH && request.method === "PUT") return saveToken(store, request);
  if (path === TOKEN_TEST_PATH && request.method === "POST") return testToken(store, request);
  return null;
}

/** What GitHub answers while nothing is under way: the stored connection, or nothing. */
function storedConnection(store: IntegrationStore): GitHubConnect {
  const row = find(store, GITHUB);
  const { github } = store;
  if (!row || row.st === "none") return { state: "idle", installations: [] };
  const oauth = github.mode === "oauth";
  return {
    state: "connected",
    mode: github.mode,
    login: github.login,
    ...(oauth ? { installUrl: INSTALL_URL } : {}),
    installations: oauth ? github.installations : [],
  };
}

function startConnect(store: IntegrationStore): Response {
  if (store.github.startRefusal) {
    return errorAnswer(STATUS.unavailable, "unavailable", store.github.startRefusal);
  }
  const flow: GitHubConnect = {
    state: "pending",
    userCode: store.github.userCode,
    verificationUri: VERIFICATION_URI,
    expiresAt: new Date(store.nowMs() + CODE_LIFETIME_MS).toISOString(),
    installations: [],
  };
  store.github.flow = flow;
  return jsonAnswer(flow);
}

/** A sign-in that reaches "connected" is stored and tested, as the daemon does the moment it does. */
function adopt(store: IntegrationStore, answer: GitHubConnect): void {
  const { github } = store;
  const row = find(store, GITHUB);
  if (answer.state !== "connected" || !row) {
    github.flow = answer;
    return;
  }
  github.flow = null;
  github.mode = "oauth";
  github.login = answer.login ?? github.login;
  github.installations = structuredClone(answer.installations);
  row.st = "connected";
  runTest(store, row, store.nowMs());
}

function readConnect(store: IntegrationStore): Response {
  const next = store.github.script.shift();
  if (next) adopt(store, next);
  return jsonAnswer(next ?? store.github.flow ?? storedConnection(store));
}

function cancelConnect(store: IntegrationStore): Response {
  if (store.github.flow?.state === "pending") store.github.flow = null;
  return jsonAnswer(store.github.flow ?? storedConnection(store));
}

function saveToken(store: IntegrationStore, request: FakeRequest): Response {
  const row = find(store, GITHUB);
  if (!row) return notFound();
  const refusal = store.refuseSave(GITHUB, bodyOf(request));
  if (refusal) return errorAnswer(STATUS.badRequest, "invalid_argument", refusal);
  store.github.flow = null;
  store.github.mode = "token";
  row.st = "connected";
  runTest(store, row, store.nowMs());
  return listAnswer(store);
}

/** The stateless test: what GitHub says to a token, with nothing saved and no cooldown. */
function testToken(store: IntegrationStore, request: FakeRequest): Response {
  const row = find(store, GITHUB);
  if (!row) return notFound();
  const refusal = store.refuseSave(GITHUB, bodyOf(request));
  const checks: TestCheck[] = refusal
    ? [
        {
          name: "Token",
          state: "failed",
          message: refusal,
          fix: "Make a new token on GitHub and paste it here.",
        },
      ]
    : store.checks(row);
  return jsonAnswer({
    connectionId: GITHUB,
    checks,
    ok: !checks.some((check) => check.state === "failed"),
    ranAt: new Date(store.nowMs()).toISOString(),
  });
}
