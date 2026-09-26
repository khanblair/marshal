/**
 * The saved view routes of the fake daemon (docs/backend-checklist.md B2.5). Each answers the way
 * the daemon's own handler does, including `saved_view.updated`, which carries the project's whole
 * list, and the quiet clearing of a deleted view from the preferences of the person who had it in
 * use, which the daemon's database does without a `me.updated`.
 */
import type { CreateSavedViewRequest, SavedView, UpdateSavedViewRequest } from "@marshal/protocol";
import { emptyAnswer, errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";
import type { MeStore } from "./fake-me";

const STATUS = {
  ok: 200,
  created: 201,
  badRequest: 400,
  notFound: 404,
  conflict: 409,
  unprocessable: 422,
};
const ID_DIGITS = 18;
const MAX_NAME_CHARS = 60;
const MAX_VIEWS = 50;

/** A saved view the fake daemon holds: the golden one, with the given fields changed. */
export const wireSavedView = (
  fields: Partial<SavedView> & { id: string; projectId: string },
): SavedView => ({
  ...golden<SavedView>("saved-view"),
  ...fields,
});

/** How many ids the fake daemon has handed out, so no two views share one. */
let made = 0;

const freshId = (): string => `01M3VIEW${String(++made).padStart(ID_DIGITS, "0")}`;

const sameName = (a: string, b: string): boolean =>
  a.trim().toLowerCase() === b.trim().toLowerCase();

const notFound = () =>
  errorAnswer(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that saved view. It may have been removed.",
  );

const noProject = () =>
  errorAnswer(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that project. It may have been removed.",
  );

function bodyOf<T>(request: FakeRequest): T {
  try {
    return (request.body ? JSON.parse(request.body) : {}) as T;
  } catch {
    return {} as T;
  }
}

/** The project's views in the order the daemon lists them: the one saved longest ago first. */
const viewsOf = (store: MeStore, pid: string): SavedView[] =>
  store.savedViews.filter((view) => view.projectId === pid);

/** Tells every device the project's views as they are now. */
function announce(store: MeStore, pid: string): void {
  store.publish(`project:${pid}`, "saved_view.updated", {
    projectId: pid,
    views: structuredClone(viewsOf(store, pid)),
  });
}

const refuse = (message: string, status = STATUS.badRequest) =>
  errorAnswer(status, "invalid_argument", message);

/** The sentence for a name the daemon refuses, or null. */
function nameRefusal(name: string): string | null {
  if (!name) return "Give the view a name.";
  if ([...name].length > MAX_NAME_CHARS) return "Saved view names can have at most 60 characters.";
  return null;
}

function createView(store: MeStore, pid: string, request: FakeRequest): Response {
  const body = bodyOf<CreateSavedViewRequest>(request);
  const name = (body.name ?? "").trim();
  const refusal = nameRefusal(name);
  if (refusal) return refuse(refusal);
  const now = store.now();
  const same = viewsOf(store, pid).find((view) => sameName(view.name, name));
  if (!same && viewsOf(store, pid).length >= MAX_VIEWS) {
    return refuse(
      "A project can have at most 50 saved views. Delete one and try again.",
      STATUS.unprocessable,
    );
  }
  // A name that is used replaces that view: it keeps its id and its first save time, and moves to the end.
  const view: SavedView = wireSavedView({
    id: same?.id ?? freshId(),
    projectId: pid,
    name,
    filters: body.filters ?? [],
    swimlane: body.swimlane || "none",
    createdAt: same?.createdAt ?? now,
    updatedAt: now,
  });
  if (same) store.savedViews.splice(store.savedViews.indexOf(same), 1);
  store.savedViews.push(view);
  announce(store, pid);
  return jsonAnswer(view, same ? STATUS.ok : STATUS.created);
}

function updateView(store: MeStore, view: SavedView, request: FakeRequest): Response {
  const body = bodyOf<UpdateSavedViewRequest>(request);
  if (body.name !== undefined) {
    const name = body.name.trim();
    const refusal = nameRefusal(name);
    if (refusal) return refuse(refusal);
    const clash = viewsOf(store, view.projectId).some(
      (other) => other.id !== view.id && sameName(other.name, name),
    );
    if (clash)
      return refuse("This project already has a saved view with that name.", STATUS.conflict);
    view.name = name;
  }
  if (body.filters !== undefined) view.filters = body.filters;
  if (body.swimlane !== undefined) view.swimlane = body.swimlane;
  view.updatedAt = store.now();
  // The list is in the order of the last save, so a changed view moves to the end.
  store.savedViews.splice(store.savedViews.indexOf(view), 1);
  store.savedViews.push(view);
  announce(store, view.projectId);
  return jsonAnswer(view);
}

/**
 * Deletes a view. A person who had it in use loses it without a `me.updated`, because the
 * database clears it (`ON DELETE SET NULL`) and the daemon publishes only the new list.
 */
function deleteView(store: MeStore, view: SavedView): Response {
  store.savedViews.splice(store.savedViews.indexOf(view), 1);
  const prefs = store.preferences.projects[view.projectId];
  if (prefs?.savedViewId === view.id) prefs.savedViewId = null;
  announce(store, view.projectId);
  return emptyAnswer();
}

/** One request under a project's saved views or to one saved view, or undefined when it is neither. */
export function answerSavedViewRoute(
  store: MeStore,
  request: FakeRequest,
  projectExists: (id: string) => boolean,
): Response | undefined {
  const path = request.url.replace(/^https?:\/\/[^/]+/, "").split("?")[0] ?? "";
  const [, kind, first, second] = path
    .split("/")
    .filter(Boolean)
    .map((segment) => decodeURIComponent(segment));
  if (kind === "projects" && first && second === "saved-views") {
    if (request.method !== "GET" && request.method !== "POST") return undefined;
    if (!projectExists(first)) return noProject();
    if (request.method === "POST") return createView(store, first, request);
    return jsonAnswer({ projectId: first, views: viewsOf(store, first), serverTime: store.now() });
  }
  if (kind !== "saved-views" || !first) return undefined;
  const view = store.savedViews.find((one) => one.id === first);
  if (!view) return notFound();
  if (request.method === "PATCH") return updateView(store, view, request);
  if (request.method === "DELETE") return deleteView(store, view);
  return undefined;
}
