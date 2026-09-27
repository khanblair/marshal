/**
 * The connection routes of the fake daemon (docs/backend-checklist.md B6.1 and B6.7, section S29a).
 * Every route answers the way the daemon's own handler does: the list carries every connection
 * Marshal knows, whether or not it is set up; a save stores the App's settings, tests the connection
 * as part of the same call, and answers the whole list; a test asked for inside the cooldown is
 * refused with how long to wait; and nothing secret ever comes back.
 *
 * The connections of later phases are listed and read as not connected, exactly as the daemon's own
 * `known()` does, so a cutover does not renumber anything. Nothing here reaches GitHub: the test's
 * checks are made up, because what the frontend needs from these routes is the shape of the answer.
 *
 * A row's status and its one sentence are not stored, they are derived from the last test the way
 * `integrations.go`'s `rowToWire` derives them: the failed check's fix, the passing test's Summary
 * check, or the daemon's own "run the test" when nothing has tested it yet. A row is "saved" here
 * when its status is not `none`, which is what the daemon stores a config and a secret for.
 */
import type {
  Integration,
  IntegrationList,
  IntegrationStatus,
  TestCheck,
  TestResult,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";

const STATUS = { ok: 200, badRequest: 400, notFound: 404, conflict: 409 };

/** The daemon's cooldown between two tests of one connection (connectiontest.DefaultCooldown). */
const DEFAULT_COOLDOWN_MS = 5000;
const MS_PER_SECOND = 1000;
const GITHUB = "github";
const SAVE_PATH = /^\/v1\/integrations\/([^/]+)$/;
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
 * Every connection the daemon knows, in the order the settings screen shows them, with the kind each
 * one's test is filed under. It mirrors `daemon/internal/integrations/integrations.go`'s `known()`.
 */
const KNOWN: readonly { id: string; kind: string }[] = [
  { id: GITHUB, kind: "github" },
  { id: "trello", kind: "trello" },
  { id: "gcal", kind: "calendar" },
  { id: "gmail", kind: "gmail" },
  { id: "telegram", kind: "telegram" },
  { id: "discord", kind: "discord" },
  { id: "obsidian", kind: "obsidian" },
];

/** The connections and what is stored for them, as the fake daemon holds them. */
export interface IntegrationStore {
  /**
   * The connections it knows, in the daemon's order. A row whose status is not `none` has a config
   * and a secret stored, and its sentence is what it shows until a test replaces it.
   */
  rows: Integration[];
  /** When each connection was last tested, in ms. No entry means it was never tested. */
  testedAt: Record<string, number>;
  /** The last test's own result, by connection id. */
  lastTest: Record<string, TestResult>;
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
  /** Refuses a save with its own sentence, as a test may need. */
  refuseSave?: (id: string, body: Record<string, unknown>) => string | undefined;
  /** The checks a test answers with. A passing GitHub test by default. */
  checks?: (row: Integration) => TestCheck[];
  /** How long a connection waits between tests, in ms. The daemon's 5s by default. */
  cooldownMs?: number;
  /** Its clock, in ms. `Date.now` by default. */
  nowMs?: () => number;
}

/** The rows the daemon lists before anything is set up: one per known connection, none connected. */
function knownRows(): Integration[] {
  return KNOWN.map(({ id, kind }) => ({ id, kind, st: "none" as const, detail: "" }));
}

export function createIntegrationStore(options: FakeIntegrationOptions = {}): IntegrationStore {
  return {
    rows: structuredClone([...(options.integrations ?? knownRows())]),
    testedAt: {},
    lastTest: {},
    refuseSave: options.refuseSave ?? defaultRefusal,
    checks: options.checks ?? githubChecks,
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
  if (row.st === "none") return { id: row.id, kind: row.kind, st: "none", detail: "" };
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

/** The one sentence under a row: the failed check's fix, the passing test's summary, or the reminder. */
function detailFor(row: Integration, last: TestResult | undefined): string {
  if (!last) return row.detail || UNTESTED_DETAIL;
  const failed = last.checks.find((check) => check.state === "failed");
  if (failed) return failed.fix ?? failed.message;
  const summary = last.checks.find((check) => check.name === SUMMARY);
  return summary ? summary.message : "Connected.";
}

function saveConnection(store: IntegrationStore, id: string, request: FakeRequest): Response {
  // Only the GitHub App has a save shape today, and an id with no save shape is refused rather than
  // silently accepted, exactly as the daemon's `saveIntegration` refuses it.
  if (id !== GITHUB) return notFound();
  const row = find(store, id);
  if (!row) return notFound();
  const body = bodyOf(request);
  const refusal = store.refuseSave(id, body);
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

/** The App's four values must all be there: the key alone, or the secret alone, is no connection. */
function defaultRefusal(_id: string, body: Record<string, unknown>): string | undefined {
  const appId = Number(body.appId ?? 0);
  const installationId = Number(body.installationId ?? 0);
  const privateKey = typeof body.privateKey === "string" ? body.privateKey.trim() : "";
  const webhookSecret = typeof body.webhookSecret === "string" ? body.webhookSecret.trim() : "";
  if (
    !Number.isInteger(appId) ||
    appId <= 0 ||
    !Number.isInteger(installationId) ||
    installationId <= 0
  ) {
    return "The App id and the installation id are both numbers.";
  }
  if (!privateKey.includes("PRIVATE KEY")) {
    return "That does not look like a private key. Paste the file GitHub generated, whole.";
  }
  if (webhookSecret === "") return "The webhook secret is missing.";
  return undefined;
}

export type { IntegrationList };
