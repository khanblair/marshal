/**
 * The role routes of the fake daemon (docs/backend-checklist.md B5.1, section S27). Every route
 * answers the way the daemon's own handler does: a role is addressed by its name, a name another
 * role already has is a conflict rather than a quiet replace, a role Marshal ships is refused a
 * delete and reset instead, a rename changes the role itself rather than one project's view of it,
 * and every route but the read of one role answers with the whole list, so a screen redraws itself
 * from one answer whatever changed.
 *
 * The two routes that change one project's view take the project from the query, because the roles
 * themselves are global and the project only changes how they read. Without it every role reports
 * as not overridden, which is what a caller that is not looking at a project should see.
 *
 * The list is seeded from the golden `role-list` the Go tests wrote, so a screen test reads the
 * shape the real daemon sends. The flag the golden carries came from a fixture that was asked about
 * a project, so it is cleared here: a project keeping its own version of a role is what makes the
 * flag true, and this store starts with no project keeping one.
 */
import type { Role, RoleList, RoleSpec } from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";

const STATUS = { badRequest: 400, notFound: 404, conflict: 409, refused: 422 };

/** The daemon's own ceilings (roles/service.go), repeated so a name it refuses is refused here too. */
const MAX_ROLE_NAME_CHARS = 60;
const MAX_LIMIT_VALUE = 1_000_000;

const LIST_PATH = "/v1/roles";
const ONE_PATH = /^\/v1\/roles\/([^/]+)$/;
const RESET_PATH = /^\/v1\/roles\/([^/]+)\/reset$/;
const OVERRIDE_PATH = /^\/v1\/roles\/([^/]+)\/override$/;

/** The roles and the projects that keep their own version of them, as the fake daemon holds them. */
export interface RolesStore {
  /** Every role it holds now, Marshal's starters first, then the roles a person made. */
  rows: Role[];
  /** The body of each role a project keeps its own version of, by role id then project id. */
  overrides: Record<string, Record<string, RoleSpec>>;
  /** The number the next role's id is made from. Ids are opaque, so no screen reads them. */
  seq: number;
  /** Its clock, as the ISO string `serverTime` is written from. */
  now: () => string;
}

export interface FakeRoleOptions {
  /** The roles it starts with. The golden list by default, with no project keeping one. */
  roles?: readonly Role[];
  /** Its clock, as an ISO string. The wall clock by default. */
  now?: () => string;
}

export function createRolesStore(options: FakeRoleOptions = {}): RolesStore {
  const rows = structuredClone([...(options.roles ?? golden<RoleList>("role-list").roles)]);
  for (const row of rows) row.overridden = false;
  return {
    rows,
    overrides: {},
    seq: rows.length,
    now: options.now ?? (() => new Date().toISOString()),
  };
}

/** Answers one role route, or null when the request is not one. */
export function answerRoleRoute(
  store: RolesStore,
  request: FakeRequest,
  projectExists: (id: string) => boolean,
): Response | null {
  const { path, project } = partsOf(request);
  if (path === LIST_PATH) {
    if (request.method === "GET") return listAnswer(store, project);
    if (request.method === "POST") return createRole(store, project, request);
    return null;
  }
  // The two routes that carry an extra segment are tried first, so a role whose name is "reset"
  // cannot take one of their addresses.
  const reset = RESET_PATH.exec(path);
  if (reset && request.method === "POST") {
    return resetRole(store, decodeURIComponent(reset[1] ?? ""), project);
  }
  const override = OVERRIDE_PATH.exec(path);
  if (override && request.method === "PUT") {
    return setOverride(
      store,
      decodeURIComponent(override[1] ?? ""),
      project,
      request,
      projectExists,
    );
  }
  const one = ONE_PATH.exec(path);
  if (!one) return null;
  const name = decodeURIComponent(one[1] ?? "");
  if (request.method === "GET") return oneAnswer(store, name, project);
  if (request.method === "PATCH") return updateRole(store, name, request);
  if (request.method === "DELETE") return removeRole(store, name, project);
  return null;
}

/** The path and the project a request names, as the routes read them. */
function partsOf(request: FakeRequest): { path: string; project: string } {
  const url = new URL(request.url, "http://fake-daemon");
  return { path: url.pathname, project: url.searchParams.get("project") ?? "" };
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const asText = (value: unknown): string => (typeof value === "string" ? value : "");

const asList = (value: unknown): string[] =>
  Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];

const asCount = (value: unknown): number =>
  typeof value === "number" && Number.isFinite(value) ? Math.trunc(value) : 0;

function bodyOf(request: FakeRequest): Record<string, unknown> {
  try {
    return request.body ? (JSON.parse(request.body) as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

const refuse = (status: number, code: string, message: string): Response =>
  errorAnswer(status, code, message);

/** The daemon's own sentence for a role that is not there (protocol.NotFound("role")). */
const notFoundRole = (): Response =>
  refuse(STATUS.notFound, "not_found", "Marshal cannot find that role. It may have been removed.");

const notFoundProject = (): Response =>
  refuse(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that project. It may have been removed.",
  );

/** The daemon's own sentence for a name that is taken (roles/errors.go). */
const conflictName = (name: string): Response =>
  refuse(
    STATUS.conflict,
    "conflict",
    `A role called "${name}" already exists. Choose another name.`,
  );

/**
 * A name that cannot be a role's, in the daemon's own words, or null when it may be one. A role is
 * addressed by its name in a route, so a slash would split the address in two.
 */
function nameRefusal(name: string): Response | null {
  if (name === "") return refuse(STATUS.badRequest, "invalid_argument", "Give the role a name.");
  if ([...name].length > MAX_ROLE_NAME_CHARS) {
    return refuse(
      STATUS.badRequest,
      "invalid_argument",
      `Role names can have at most ${MAX_ROLE_NAME_CHARS} characters.`,
    );
  }
  if (name.includes("/")) {
    return refuse(STATUS.badRequest, "invalid_argument", "Role names cannot contain a slash.");
  }
  return null;
}

/**
 * One request's role body, with the defaults the daemon's own decoding gives it, or what to answer
 * instead. The field-length ceilings the daemon also checks are not repeated here: they stop a
 * runaway paste, and a role longer than one of them is not a role a screen can write.
 */
function readSpec(value: unknown): RoleSpec | Response {
  const spec = isRecord(value) ? value : {};
  const limits = isRecord(spec.limits) ? spec.limits : {};
  const three = {
    time: asCount(limits.time),
    cost: asCount(limits.cost),
    rounds: asCount(limits.rounds),
  };
  for (const each of [three.time, three.cost, three.rounds]) {
    if (each < 0) {
      return refuse(STATUS.badRequest, "invalid_argument", "A role's limits cannot be negative.");
    }
    if (each > MAX_LIMIT_VALUE) {
      return refuse(
        STATUS.badRequest,
        "invalid_argument",
        `A role's limit can be at most ${MAX_LIMIT_VALUE}.`,
      );
    }
  }
  return {
    skills: asList(spec.skills),
    mcp: asList(spec.mcp),
    limits: three,
    backup: asText(spec.backup),
    desc: asText(spec.desc),
    agent: asText(spec.agent),
    model: asText(spec.model),
    think: asText(spec.think),
    perm: asText(spec.perm),
    strength: asText(spec.strength),
    instr: asText(spec.instr),
  };
}

const find = (store: RolesStore, name: string): Role | undefined =>
  store.rows.find((row) => row.name === name);

/**
 * One role as the list shows it: the role's own body, with the flag set when the project being
 * looked at keeps its own version of it. The daemon does not swap the override's body in either
 * (roles/service.go's toRoles), because the roles a screen shows are the roles themselves.
 */
function shown(store: RolesStore, row: Role, project: string): Role {
  const kept = store.overrides[row.id]?.[project];
  return { ...row, overridden: project !== "" && kept !== undefined };
}

function listAnswer(store: RolesStore, project: string): Response {
  return jsonAnswer({
    roles: store.rows.map((row) => shown(store, row, project)),
    serverTime: store.now(),
  });
}

function oneAnswer(store: RolesStore, name: string, project: string): Response {
  const row = find(store, name);
  if (!row) return notFoundRole();
  return jsonAnswer(shown(store, row, project));
}

function createRole(store: RolesStore, project: string, request: FakeRequest): Response {
  const body = bodyOf(request);
  const name = asText(body.name).trim();
  const bad = nameRefusal(name);
  if (bad) return bad;
  const spec = readSpec(body.spec);
  if (spec instanceof Response) return spec;
  if (find(store, name)) return conflictName(name);
  store.seq += 1;
  store.rows.push({ id: `role-${store.seq}`, name, starter: false, overridden: false, spec });
  return listAnswer(store, project);
}

function updateRole(store: RolesStore, name: string, request: FakeRequest): Response {
  const row = find(store, name);
  if (!row) return notFoundRole();
  const body = bodyOf(request);
  const wantsName = typeof body.name === "string";
  const wanted = wantsName ? asText(body.name).trim() : "";
  if (wantsName) {
    const bad = nameRefusal(wanted);
    if (bad) return bad;
    const other = find(store, wanted);
    if (other && other !== row) return conflictName(wanted);
  }
  if (isRecord(body.spec)) {
    const spec = readSpec(body.spec);
    if (spec instanceof Response) return spec;
    row.spec = spec;
  }
  if (wantsName) row.name = wanted;
  // A change answers the list asked for with no project (roles/service.go's UpdateRole), so every
  // role reads as not overridden here however the caller asked.
  return listAnswer(store, "");
}

function removeRole(store: RolesStore, name: string, project: string): Response {
  const at = store.rows.findIndex((row) => row.name === name);
  if (at < 0) return notFoundRole();
  const row = store.rows[at] as Role;
  if (row.starter) {
    return refuse(
      STATUS.refused,
      "refused",
      `${row.name} is one of Marshal's own roles, so it cannot be deleted. Reset it instead.`,
    );
  }
  store.rows.splice(at, 1);
  delete store.overrides[row.id];
  return listAnswer(store, project);
}

function resetRole(store: RolesStore, name: string, project: string): Response {
  if (project === "") {
    return refuse(
      STATUS.badRequest,
      "invalid_argument",
      "A reset needs the project to reset it for.",
    );
  }
  const row = find(store, name);
  if (!row) return notFoundRole();
  // Clearing an override a project does not keep is not an error: there is nothing to remove, which
  // is the answer a caller that reset every project it holds will meet for most of them.
  const kept = store.overrides[row.id];
  if (kept) delete kept[project];
  return listAnswer(store, project);
}

function setOverride(
  store: RolesStore,
  name: string,
  project: string,
  request: FakeRequest,
  projectExists: (id: string) => boolean,
): Response {
  if (project === "") {
    return refuse(
      STATUS.badRequest,
      "invalid_argument",
      "An override needs the project it is for.",
    );
  }
  // The override route is the one that insists the project is really there (roles/service.go); the
  // reset route beside it only insists that one was named.
  if (!projectExists(project)) return notFoundProject();
  const row = find(store, name);
  if (!row) return notFoundRole();
  const spec = readSpec(bodyOf(request));
  if (spec instanceof Response) return spec;
  const kept = store.overrides[row.id] ?? {};
  store.overrides[row.id] = kept;
  kept[project] = spec;
  return listAnswer(store, project);
}
