import {
  type CreateSavedViewRequest,
  EventTypeProjectRemoved,
  EventTypeSavedViewUpdated,
  type Event as WireEvent,
  type SavedView as WireSavedView,
} from "@marshal/protocol";
import { batch } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";
import type { State } from "~/mock/state-types";
import type { SavedView } from "~/mock/types";
import { toStoredView, toViewList, toWireFilters } from "./me-mapper";
import { noteSavedViewGone } from "./preferences";
import type { Syncer } from "./syncer";

/**
 * Section S6a: each project's saved views, and the `saved_view.updated` event of its topic.
 *
 * The daemon owns the views, and the menu lists them with the client's own "All cards" first (see
 * `toViewList`). The event carries the project's whole list, so a view another device saved, changed,
 * or deleted arrives the same way as one saved here, and applying it twice changes nothing.
 */

interface ProjectViews {
  projectId: string;
  views: WireSavedView[];
}

/** A view gone from the list ends its use, and a renamed one keeps it under its new name. */
function settleViewInUse(
  S: State,
  pid: string,
  before: SavedView[] | undefined,
  list: SavedView[],
) {
  const name = S.savedView[pid] ?? null;
  if (!name) return null;
  const id = before?.find((view) => view.name === name)?.id;
  // The daemon's view is followed by its id, so renaming it does not end its use. The client's own
  // "All cards" has no id and stays while the list still has a view of that name.
  if (id) return list.find((view) => view.id === id)?.name ?? null;
  return list.some((view) => view.name === name) ? name : null;
}

/** Makes the project's menu list the daemon's, and keeps the view in use only while it is still there. */
function applyViewList(ctx: Ctx, pid: string, views: readonly WireSavedView[]): void {
  const { S } = ctx;
  batch(() => {
    const before = S.savedViews[pid];
    const list = toViewList(views);
    const inUse = settleViewInUse(S, pid, before, list);
    S.savedViews[pid] = list;
    if ((S.savedView[pid] ?? null) !== inUse) {
      S.savedView[pid] = inUse;
      // The daemon cleared it with the view, and said so only here, so the preferences follow.
      if (inUse === null) noteSavedViewGone(ctx, pid);
    }
  });
}

/** Puts one saved view into the project's list, at the end, in place of any view of the same id or name. */
function putView(S: State, pid: string, view: SavedView): void {
  const name = view.name.trim().toLowerCase();
  const others = (S.savedViews[pid] ?? []).filter(
    (other) => other.id !== view.id && other.name.trim().toLowerCase() !== name,
  );
  S.savedViews[pid] = [...others, view];
}

/**
 * Saves the filters and swimlane on screen as a view of the given name, on the daemon, and puts it
 * in use. A name the project already has replaces that view. The daemon's refusal (a name that is
 * too long, or the limit of 50) is shown as its own sentence by `optimistic`, and nothing changes.
 */
export async function saveView(ctx: Ctx, name: string): Promise<void> {
  const { S } = ctx;
  const pid = S.route.pid;
  const api = ctx.env.data?.api;
  if (!pid || !name || !api) return;
  const request: CreateSavedViewRequest = {
    name,
    filters: toWireFilters(S.filters[pid] ?? []),
    swimlane: S.swim[pid] ?? "none",
  };
  try {
    const saved = await ctx.optimistic({
      apply: () => undefined,
      request: () => api.createSavedView(pid, request),
      rollback: () => undefined,
    });
    batch(() => {
      const view = toStoredView(saved);
      putView(S, pid, view);
      S.savedView[pid] = view.name;
      S.menu = null;
      toast(ctx, "View saved");
    });
  } catch {
    // The daemon refused, and `optimistic` showed its sentence.
  }
}

function onViewEvent(ctx: Ctx, event: WireEvent): void {
  if (!isRecord(event.data)) return;
  if (event.type === EventTypeSavedViewUpdated) {
    const { projectId, views } = event.data;
    if (typeof projectId === "string" && Array.isArray(views)) {
      applyViewList(ctx, projectId, views as WireSavedView[]);
    }
    return;
  }
  // A removed project takes its saved views with it on the daemon, and says so with this alone.
  if (event.type === EventTypeProjectRemoved && typeof event.data.projectId === "string") {
    const pid = event.data.projectId;
    delete ctx.S.savedViews[pid];
    delete ctx.S.savedView[pid];
  }
}

export const savedViewsSyncer: Syncer<ProjectViews[]> = {
  section: "S6a",
  topics: [],
  projectTopics: (projectID) => [`project:${projectID}`],
  async load(api: ApiClient, ctx: Ctx) {
    const lists = await Promise.all(
      ctx.S.projects.map((project) => api.listSavedViews(project.id)),
    );
    return lists.map((list) => ({ projectId: list.projectId, views: list.views }));
  },
  apply(ctx, lists) {
    batch(() => {
      for (const list of lists) applyViewList(ctx, list.projectId, list.views);
    });
  },
  onEvent: onViewEvent,
};
