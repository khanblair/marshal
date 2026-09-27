/**
 * The limits routes of the fake daemon (sections S19b and S26b). Every route answers the way the
 * daemon's own handler does: the whole list of ceilings that are set, global ones first, with a
 * ceiling the daemon does not have simply absent - which is how a fresh install answers.
 *
 * The list is seeded from the golden `limit-list` the Go tests wrote, so a screen test reads the
 * same numbers the real daemon sends.
 */
import type { Limit, LimitKind, LimitList } from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";

const STATUS = { badRequest: 400, notFound: 404 };
const LIST_PATH = "/v1/limits";
const ONE_PATH = /^\/v1\/limits\/([^/]+)\/([^/]+)$/;

/** Every kind the routes know, in the order the daemon lists them. */
const KINDS: readonly LimitKind[] = ["cost-day", "cost-month", "awake"];

/** The ceilings and their values, as the fake daemon holds them. */
export interface LimitsStore {
  /** One entry per ceiling that is set. A scope with no ceiling has no entry. */
  limits: Limit[];
}

export interface FakeLimitOptions {
  /** The ceilings it starts with. The golden list by default. */
  limits?: readonly Limit[];
}

export function createLimitsStore(options: FakeLimitOptions = {}): LimitsStore {
  return {
    limits: structuredClone([...(options.limits ?? golden<LimitList>("limit-list").limits)]),
  };
}

/** Answers one limits route, or null when the request is not one. */
export function answerLimitRoute(store: LimitsStore, request: FakeRequest): Response | null {
  const path = request.url.replace(/^https?:\/\/[^/]+/, "");
  if (path === LIST_PATH && request.method === "GET") return listAnswer(store);
  const one = ONE_PATH.exec(path);
  if (!one) return null;
  const scope = decodeURIComponent(one[1] ?? "");
  const kind = decodeURIComponent(one[2] ?? "");
  if (!isKind(kind)) {
    return errorAnswer(STATUS.notFound, "not_found", "Marshal has no limit of that kind.");
  }
  if (request.method === "PUT") return setLimit(store, scope, kind, request);
  if (request.method === "DELETE") return removeLimit(store, scope, kind);
  return null;
}

const isKind = (value: string): value is LimitKind => (KINDS as readonly string[]).includes(value);

const listAnswer = (store: LimitsStore): Response => jsonAnswer({ limits: store.limits });

function bodyOf(request: FakeRequest): Record<string, unknown> {
  try {
    return request.body ? (JSON.parse(request.body) as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

function setLimit(
  store: LimitsStore,
  scope: string,
  kind: LimitKind,
  request: FakeRequest,
): Response {
  const value = bodyOf(request).value;
  if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) {
    return errorAnswer(STATUS.badRequest, "invalid_argument", "A limit must be more than zero.");
  }
  const row: Limit = { scope, kind, value: Math.round(value) };
  const at = store.limits.findIndex((limit) => limit.scope === scope && limit.kind === kind);
  if (at >= 0) store.limits[at] = row;
  else store.limits.push(row);
  return listAnswer(store);
}

function removeLimit(store: LimitsStore, scope: string, kind: LimitKind): Response {
  const at = store.limits.findIndex((limit) => limit.scope === scope && limit.kind === kind);
  if (at >= 0) store.limits.splice(at, 1);
  return listAnswer(store);
}
