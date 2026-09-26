/**
 * The person's routes of the fake daemon (docs/backend-checklist.md B2.2 and B2.5): the profile and
 * its avatar, the users, the progress, and the preferences. They answer the way the daemon's own
 * handlers do, with the refusal sentences it uses, and publish `me.updated` after a change that
 * changed something, so a test that follows two devices sees one change reach the other.
 *
 * The shapes come from the golden files, which the Go tests wrote from the real wire types. The
 * saved views are in `fake-saved-views.ts`.
 */
import type {
  Preferences,
  Profile,
  Progress,
  ProjectPreferences,
  SavedView,
  UpdatePreferencesRequest,
  UpdateProfileRequest,
  UpdateProgressRequest,
  User,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";

const STATUS = { badRequest: 400, notFound: 404 };
const KIB = 1024;
const MAX_AVATAR_BYTES = 2 * KIB * KIB;
const MAX_NAME_CHARS = 100;
const MAX_QUERY_CHARS = 200;
const AVATAR_TYPES = ["image/png", "image/jpeg", "image/webp"];
/** A one-pixel PNG: what the fake serves for any avatar, whatever was uploaded. */
const PIXEL = Uint8Array.from(
  atob(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==",
  ),
  (char) => char.charCodeAt(0),
);

/** What the fake daemon holds for the person, and how it tells the event stream about a change. */
export interface MeStore {
  profile: Profile;
  progress: Progress;
  preferences: Preferences;
  /** Every project's saved views, oldest save first within a project. */
  savedViews: SavedView[];
  /** The type and size of the avatar uploaded, or null when there is none. */
  avatar: { type: string; size: number } | null;
  /** True when the daemon runs in dev mode, which is what the first-launch reset needs. */
  dev: boolean;
  publish: (topic: string, type: string, data: unknown) => void;
  now: () => string;
}

/** The person a fake daemon starts with: the golden profile, with no avatar. */
export const wireProfile = (fields: Partial<Profile> = {}): Profile => ({
  ...golden<Profile>("profile"),
  avatarUrl: null,
  ...fields,
});

export const emptyPreferences = (): Preferences => ({
  theme: "system",
  listColumns: {},
  sort: { agents: null, list: null },
  projects: {},
});

/**
 * The preferences with the row the daemon holds for the prototype's monorepo (`mobile`) once it has
 * been sent the package swimlane the client starts it on. The client sends that once, at load, for a
 * monorepo the daemon has no row for (the daemon's own default swimlane is none), so a test that is
 * not about that starts from this and sees nothing sent at load.
 */
export function withMobileRow(preferences: Preferences = emptyPreferences()): Preferences {
  const projects = { ...preferences.projects };
  projects.mobile ??= {
    lastView: "board",
    filters: [],
    query: "",
    swimlane: "package",
    collapsedLanes: [],
    showAllDone: false,
    savedViewId: null,
  };
  return { ...preferences, projects };
}

export const pendingProgress = (): Progress => ({
  onboarding: { status: "pending", step: 0, finishedAt: null },
  tutorial: { status: "pending", finishedAt: null },
});

const refuse = (message: string, status = STATUS.badRequest) =>
  errorAnswer(status, status === STATUS.notFound ? "not_found" : "invalid_argument", message);

const noProject = () =>
  refuse("Marshal cannot find that project. It may have been removed.", STATUS.notFound);

/** The whole person as `me.updated` carries it. A copy, so a later change cannot reach an event already sent. */
function announceMe(store: MeStore): void {
  store.publish(
    "me",
    "me.updated",
    structuredClone({
      profile: store.profile,
      preferences: store.preferences,
      progress: store.progress,
    }),
  );
}

function bodyOf<T>(request: FakeRequest): T {
  try {
    return (request.body ? JSON.parse(request.body) : {}) as T;
  } catch {
    return {} as T;
  }
}

const initialsOf = (name: string): string =>
  name
    .split(/\s+/)
    .map((word) => word[0] ?? "")
    .join("")
    .slice(0, 2)
    .toUpperCase();

const validTimeZone = (zone: string): boolean => {
  if (zone === "" || zone === "Local") return zone === "";
  try {
    new Intl.DateTimeFormat("en", { timeZone: zone });
    return true;
  } catch {
    return false;
  }
};

/** The sentence the daemon gives for a profile change it refuses, or null when it takes it. */
function profileRefusal(change: UpdateProfileRequest): string | null {
  if (change.name !== undefined) {
    const name = change.name.trim();
    if (!name) return "Enter a name. It shows on cards you comment on.";
    if ([...name].length > MAX_NAME_CHARS) return "A name can have at most 100 characters.";
  }
  if (change.email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(change.email.trim())) {
    return "That does not look like an email address. Check it and try again.";
  }
  if (change.timeZone !== undefined && !validTimeZone(change.timeZone)) {
    return "Marshal does not know that time zone. Choose one from the list.";
  }
  return null;
}

function updateProfile(store: MeStore, request: FakeRequest): Response {
  const change = bodyOf<UpdateProfileRequest>(request);
  const refusal = profileRefusal(change);
  if (refusal) return refuse(refusal);
  const before = JSON.stringify(store.profile);
  const { profile } = store;
  if (change.name !== undefined) {
    profile.name = change.name.trim();
    profile.initials = initialsOf(profile.name);
  }
  if (change.email !== undefined) profile.email = change.email.trim();
  if (change.timeZone !== undefined) profile.timeZone = change.timeZone;
  if (JSON.stringify(profile) !== before) {
    profile.updatedAt = store.now();
    announceMe(store);
  }
  return jsonAnswer(profile);
}

function avatarRefusal(request: FakeRequest): string | null {
  const image = request.raw;
  const type = request.headers["content-type"] ?? "";
  if (!(image instanceof Blob) || image.size === 0) return "Choose an image to upload.";
  if (!AVATAR_TYPES.includes(type))
    return "Marshal accepts PNG, JPEG, and WebP images. Choose one of those.";
  if (image.size > MAX_AVATAR_BYTES) return "That image is larger than 2 MB. Choose a smaller one.";
  return null;
}

function setAvatar(store: MeStore, request: FakeRequest): Response {
  const refusal = avatarRefusal(request);
  if (refusal) return refuse(refusal);
  const image = request.raw as Blob;
  store.avatar = { type: image.type, size: image.size };
  store.profile.avatarUrl = `/v1/users/${store.profile.id}/avatar?v=${Date.now()}`;
  store.profile.updatedAt = store.now();
  announceMe(store);
  return jsonAnswer(store.profile);
}

function removeAvatar(store: MeStore): Response {
  // Removing an avatar that is not there is not an error, and says nothing to the other devices.
  if (store.avatar) {
    store.avatar = null;
    store.profile.avatarUrl = null;
    store.profile.updatedAt = store.now();
    announceMe(store);
  }
  return jsonAnswer(store.profile);
}

function serveAvatar(store: MeStore, userId: string): Response {
  if (userId !== store.profile.id || !store.avatar) {
    return refuse("Marshal cannot find that avatar. It may have been removed.", STATUS.notFound);
  }
  return new Response(PIXEL, { headers: { "Content-Type": store.avatar.type } });
}

/** The users the fake lists: the person, as the users list shows them. */
const userOf = (profile: Profile): User => ({
  id: profile.id,
  name: profile.name,
  initials: profile.initials,
  avatarUrl: profile.avatarUrl ?? null,
});

function updateProgress(store: MeStore, request: FakeRequest): Response {
  const change = bodyOf<UpdateProgressRequest>(request);
  const { onboarding, tutorial } = store.progress;
  const before = JSON.stringify(store.progress);
  const finished = (status: string) => (status === "pending" ? null : store.now());
  if (change.onboarding?.step !== undefined) onboarding.step = change.onboarding.step;
  if (change.onboarding?.status) {
    onboarding.status = change.onboarding.status;
    onboarding.finishedAt = finished(onboarding.status);
  }
  if (change.tutorial?.status) {
    tutorial.status = change.tutorial.status;
    tutorial.finishedAt = finished(tutorial.status);
  }
  if (JSON.stringify(store.progress) !== before) announceMe(store);
  return jsonAnswer(store.progress);
}

function themeRefusal(theme: string | undefined): string | null {
  if (theme === undefined || ["light", "dark", "system"].includes(theme)) return null;
  return "That is not a theme Marshal knows. Choose light, dark, or system.";
}

/** The sentence for a change to one project's preferences the daemon refuses, or null. */
function projectRefusal(
  store: MeStore,
  pid: string,
  change: NonNullable<UpdatePreferencesRequest["projects"]>[string],
) {
  if (
    change.lastView !== undefined &&
    !["chat", "agents", "board", "list", "timeline", "calendar"].includes(change.lastView)
  ) {
    return "That is not a view Marshal knows. Use chat, agents, board, list, timeline, or calendar.";
  }
  if (change.query !== undefined && [...change.query].length > MAX_QUERY_CHARS) {
    return "A search can have at most 200 characters.";
  }
  if (
    change.swimlane !== undefined &&
    !["none", "role", "agent", "package", "label"].includes(change.swimlane)
  ) {
    return "That is not a swimlane Marshal knows. Group by role, agent, package, or label, or use none.";
  }
  const viewId = change.savedViewId;
  if (viewId && !store.savedViews.some((view) => view.id === viewId && view.projectId === pid)) {
    return store.savedViews.some((view) => view.id === viewId)
      ? "That saved view belongs to another project."
      : "Marshal cannot find that saved view. It may have been removed.";
  }
  return null;
}

const DEFAULT_PROJECT: ProjectPreferences = {
  lastView: "board",
  filters: [],
  query: "",
  swimlane: "none",
  collapsedLanes: [],
  showAllDone: false,
  savedViewId: null,
};

/** One project's preferences after a change: each field that is sent replaces the one that was there. */
function mergeProject(
  base: ProjectPreferences,
  change: NonNullable<UpdatePreferencesRequest["projects"]>[string],
): ProjectPreferences {
  const next = { ...base };
  if (change.lastView !== undefined) next.lastView = change.lastView;
  if (change.filters !== undefined) next.filters = change.filters;
  if (change.query !== undefined) next.query = change.query;
  if (change.swimlane !== undefined) next.swimlane = change.swimlane;
  if (change.collapsedLanes !== undefined) next.collapsedLanes = change.collapsedLanes;
  if (change.showAllDone !== undefined) next.showAllDone = change.showAllDone;
  if (change.savedViewId !== undefined) next.savedViewId = change.savedViewId || null;
  return next;
}

function updatePreferences(
  store: MeStore,
  request: FakeRequest,
  projectExists: (id: string) => boolean,
): Response {
  const change = bodyOf<UpdatePreferencesRequest>(request);
  const themeSentence = themeRefusal(change.theme);
  if (themeSentence) return refuse(themeSentence);
  for (const [pid, projectChange] of Object.entries(change.projects ?? {})) {
    const sentence = projectRefusal(store, pid, projectChange);
    if (sentence)
      return refuse(
        sentence,
        sentence.startsWith("Marshal cannot find") ? STATUS.notFound : STATUS.badRequest,
      );
    if (!projectExists(pid)) return noProject();
  }
  const before = JSON.stringify(store.preferences);
  const prefs = store.preferences;
  if (change.theme) prefs.theme = change.theme;
  Object.assign(prefs.listColumns, change.listColumns);
  if (change.sort?.agents) prefs.sort.agents = change.sort.agents;
  if (change.sort?.list) prefs.sort.list = change.sort.list;
  for (const [pid, projectChange] of Object.entries(change.projects ?? {})) {
    const existed = pid in prefs.projects;
    const merged = mergeProject(prefs.projects[pid] ?? DEFAULT_PROJECT, projectChange);
    // A project that had nothing saved and is changed to what the defaults are has nothing to keep.
    if (existed || JSON.stringify(merged) !== JSON.stringify(DEFAULT_PROJECT))
      prefs.projects[pid] = merged;
  }
  if (JSON.stringify(prefs) !== before) announceMe(store);
  return jsonAnswer(prefs);
}

/**
 * One request to the person's routes, or undefined when it is not one of them, so the caller can go
 * on to the next group of routes and end at the daemon's own not-found.
 */
export function answerMeRoute(
  store: MeStore,
  request: FakeRequest,
  projectExists: (id: string) => boolean,
): Response | undefined {
  const path = request.url.replace(/^https?:\/\/[^/]+/, "").split("?")[0] ?? "";
  const key = `${request.method} ${path}`;
  const avatar = /^GET \/v1\/users\/([^/]+)\/avatar$/.exec(key);
  if (avatar?.[1]) return serveAvatar(store, decodeURIComponent(avatar[1]));
  switch (key) {
    case "GET /v1/me":
      return jsonAnswer(store.profile);
    case "PATCH /v1/me":
      return updateProfile(store, request);
    case "POST /v1/me/avatar":
      return setAvatar(store, request);
    case "DELETE /v1/me/avatar":
      return removeAvatar(store);
    case "GET /v1/users":
      return jsonAnswer({ users: [userOf(store.profile)], serverTime: store.now() });
    case "GET /v1/me/progress":
      return jsonAnswer(store.progress);
    case "PATCH /v1/me/progress":
      return updateProgress(store, request);
    case "GET /v1/me/preferences":
      return jsonAnswer(store.preferences);
    case "PATCH /v1/me/preferences":
      return updatePreferences(store, request, projectExists);
    case "POST /v1/dev/reset-first-launch":
      return store.dev ? resetFirstLaunch(store) : undefined;
    default:
      return undefined;
  }
}

function resetFirstLaunch(store: MeStore): Response {
  store.progress = pendingProgress();
  announceMe(store);
  return jsonAnswer(store.progress);
}
