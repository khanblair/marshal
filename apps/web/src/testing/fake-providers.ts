/**
 * The provider key routes of the fake daemon (docs/backend-checklist.md B4.1-B4.6, section S28).
 * Every route answers the way the daemon's own handler does: the key itself is never answered, only
 * its masked form; a save checks the value, stores it, and tests it as part of the same call; a test
 * asked for inside the cooldown is refused with how long to wait; and every answer is the whole list
 * with each row's last test attached.
 *
 * The masking here mirrors `daemon/internal/providers/keys.go` so a fake row reads like a real one.
 * Nothing here reaches a provider: the test's checks are made up, because what the frontend needs
 * from this route is the shape of the answer, not an opinion about a key.
 */
import type { Provider, ProviderList, TestCheck, TestResult } from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";

const STATUS = { ok: 200, badRequest: 400, notFound: 404, conflict: 409 };

/** The daemon's cooldown between two tests of one connection (connectiontest.DefaultCooldown). */
const DEFAULT_COOLDOWN_MS = 5000;
const MS_PER_SECOND = 1000;
/** The daemon's own masking constants (providers/keys.go). */
const MASK_MIN = 12;
const MASK_TAIL = 4;
const MASK_PREFIX_LIMIT = 8;
const SHORT_PREFIX = 4;
const CHECK_API_KEY = "API key";
const CHECK_RATE_LIMITS = "Rate limits";
/** The providers Marshal cannot name a model for, so a test can only report what is stored. */
const UNNAMEABLE = new Set(["openrouter", "lmstudio"]);
const SAVE_PATH = /^\/v1\/providers\/([^/]+)$/;
const TEST_PATH = /^\/v1\/providers\/([^/]+)\/test$/;
const LIST_PATH = "/v1/providers";

/** The providers and their stored values, as the fake daemon holds them. */
export interface ProviderStore {
  /** The rows it holds, in the daemon's order. */
  rows: Provider[];
  /** When each provider was last tested, in ms. No entry means it was never tested. */
  testedAt: Record<string, number>;
  /** The last test's own result, by provider id. */
  lastTest: Record<string, TestResult>;
  /** The stored values, by provider id. Only the masked form is ever answered. */
  secrets: Record<string, string>;
  /** Refuses a value with its own sentence, or answers undefined to store it. */
  refuseKey: (row: Provider, key: string) => string | undefined;
  /** The checks one provider's test answers with. */
  checks: (row: Provider) => TestCheck[];
  /** How long a connection waits between tests. */
  cooldownMs: number;
  /** The daemon's clock, in ms. */
  nowMs: () => number;
}

export interface FakeProviderOptions {
  /** The rows it starts with. The golden list by default. */
  providers?: readonly Provider[];
  /** Refuses a save with its own sentence, as a test may need. */
  refuseKey?: (row: Provider, key: string) => string | undefined;
  /** The checks a test answers with. A passing test by default. */
  checks?: (row: Provider) => TestCheck[];
  /** How long a connection waits between tests, in ms. The daemon's 5s by default. */
  cooldownMs?: number;
  /** Its clock, in ms. `Date.now` by default. */
  nowMs?: () => number;
}

/** The daemon's Mask: the key's own kind prefix, an ellipsis, and its last four characters. */
export function maskSecret(secret: string): string {
  const runes = [...secret];
  if (runes.length === 0) return "";
  if (runes.length < MASK_MIN) return `…${runes[runes.length - 1]}`;
  return `${maskPrefix(runes)}…${runes.slice(-MASK_TAIL).join("")}`;
}

function maskPrefix(runes: string[]): string {
  const limit = Math.min(runes.length, MASK_PREFIX_LIMIT);
  let end = 0;
  for (let i = 0; i < limit; i += 1) if (runes[i] === "-") end = i + 1;
  if (end > 0) return runes.slice(0, end).join("");
  return runes.slice(0, Math.min(runes.length, SHORT_PREFIX)).join("");
}

/** What a screen is shown for a stored value: the address itself for a local provider, else the mask. */
function maskedValue(row: Provider, secret: string): string {
  return row.local ? secret : maskSecret(secret);
}

export function createProviderStore(options: FakeProviderOptions = {}): ProviderStore {
  const rows = structuredClone([
    ...(options.providers ?? golden<ProviderList>("provider-list").providers),
  ]);
  const store: ProviderStore = {
    rows,
    testedAt: {},
    lastTest: {},
    secrets: {},
    refuseKey: options.refuseKey ?? defaultRefusal,
    checks: options.checks ?? passingChecks,
    cooldownMs: options.cooldownMs ?? DEFAULT_COOLDOWN_MS,
    nowMs: options.nowMs ?? Date.now,
  };
  // A row that arrives already saved or invalid has something stored, so a test of it says the key
  // was accepted rather than that none is stored. The masked value stands in for the real key,
  // which this fake never has.
  for (const row of rows) if (row.masked !== "") store.secrets[row.id] = row.masked;
  return store;
}

/** Answers one provider route, or null when the request is not one. */
export function answerProviderRoute(store: ProviderStore, request: FakeRequest): Response | null {
  const path = request.url.replace(/^https?:\/\/[^/]+/, "");
  if (path === LIST_PATH && request.method === "GET") return listAnswer(store);
  const test = TEST_PATH.exec(path);
  if (test && request.method === "POST") {
    return testProvider(store, decodeURIComponent(test[1] ?? ""));
  }
  const one = SAVE_PATH.exec(path);
  if (!one) return null;
  const id = decodeURIComponent(one[1] ?? "");
  if (request.method === "PUT") return saveProvider(store, id, request);
  if (request.method === "DELETE") return removeProvider(store, id);
  return null;
}

function find(store: ProviderStore, id: string): Provider | undefined {
  return store.rows.find((row) => row.id === id);
}

const notFound = (): Response =>
  errorAnswer(STATUS.notFound, "not_found", "Marshal has no model provider with that id.");

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

function listAnswer(store: ProviderStore): Response {
  return jsonAnswer({
    providers: store.rows.map((row) => {
      const lastTest = store.lastTest[row.id];
      return lastTest ? { ...row, lastTest } : { ...row };
    }),
    serverTime: new Date(store.nowMs()).toISOString(),
  });
}

function saveProvider(store: ProviderStore, id: string, request: FakeRequest): Response {
  const row = find(store, id);
  if (!row) return notFound();
  const body = bodyOf(request);
  const key = typeof body.key === "string" ? body.key : "";
  const refusal = store.refuseKey(row, key);
  if (refusal) return errorAnswer(STATUS.badRequest, "invalid_argument", refusal);
  store.secrets[id] = key;
  row.masked = maskedValue(row, key);
  row.st = "saved";
  row.error = "";
  // The daemon tests a connection that has just changed, and does so without the cooldown
  // (connectiontest.RunAfterConnect), so a save may leave the row invalid.
  runTest(store, row, store.nowMs());
  return listAnswer(store);
}

function removeProvider(store: ProviderStore, id: string): Response {
  const row = find(store, id);
  if (!row) return notFound();
  delete store.secrets[id];
  delete store.testedAt[id];
  delete store.lastTest[id];
  row.st = "empty";
  row.masked = "";
  row.error = "";
  return listAnswer(store);
}

function testProvider(store: ProviderStore, id: string): Response {
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

function cooldownLeft(store: ProviderStore, id: string, at: number): number {
  const last = store.testedAt[id];
  if (last === undefined) return 0;
  const elapsed = at - last;
  return elapsed >= store.cooldownMs ? 0 : store.cooldownMs - elapsed;
}

/** Runs one test, saves its result, and answers it. A failed key check makes a saved row invalid. */
function runTest(store: ProviderStore, row: Provider, at: number): TestResult {
  const secret = store.secrets[row.id] ?? "";
  const checks = secret === "" ? [noKeyCheck(row)] : store.checks(row);
  const result: TestResult = {
    connectionId: row.id,
    checks,
    ok: !checks.some((check) => check.state === "failed"),
    ranAt: new Date(at).toISOString(),
  };
  store.testedAt[row.id] = at;
  store.lastTest[row.id] = result;
  const failed = checks.find((check) => check.state === "failed");
  if (failed && secret !== "") {
    row.st = "invalid";
    row.error = failed.fix || failed.message;
  }
  return result;
}

function noKeyCheck(row: Provider): TestCheck {
  return {
    name: CHECK_API_KEY,
    state: "failed",
    message: `No ${row.name} key is stored, so Marshal cannot use ${row.name}.`,
    fix: `Add a key for ${row.name} in Settings, under Provider keys.`,
  };
}

/** A test that found nothing wrong: the key worked and the provider said nothing about allowances. */
function passingChecks(row: Provider): TestCheck[] {
  if (UNNAMEABLE.has(row.id)) {
    return [
      {
        name: CHECK_API_KEY,
        state: "warning",
        message: `Marshal cannot name a model to test ${row.name} with, so it did not call ${row.name}.`,
        fix: "The key is stored. Start a chat to try it.",
      },
    ];
  }
  return [
    {
      name: CHECK_API_KEY,
      state: "passed",
      message: `${row.name} accepted the key and answered a tiny request.`,
    },
    {
      name: CHECK_RATE_LIMITS,
      state: "warning",
      message: `${row.name} did not say how many requests are left.`,
    },
  ];
}

function defaultRefusal(row: Provider, key: string): string | undefined {
  if (key.trim() !== key) return `The ${row.name} key cannot start or end with a space.`;
  if (key === "") {
    return row.local
      ? `Enter the address of the ${row.name} server.`
      : `Enter the ${row.name} API key.`;
  }
  if (row.local && !isServerAddress(key)) {
    return `The ${row.name} address is not an http address, like http://127.0.0.1:1234/v1.`;
  }
  return undefined;
}

function isServerAddress(value: string): boolean {
  try {
    const url = new URL(value);
    return (url.protocol === "http:" || url.protocol === "https:") && url.host !== "";
  } catch {
    return false;
  }
}
