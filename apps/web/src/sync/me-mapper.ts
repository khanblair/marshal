import type {
  Filter as WireFilter,
  Profile as WireProfile,
  Progress as WireProgress,
  SavedView as WireSavedView,
  SortOrder as WireSortOrder,
  User as WireUser,
} from "@marshal/protocol";
import type { Profile } from "~/mock/settings-types";
import type { Filter, Person, SavedView, SortSpec } from "~/mock/types";

/*
 * The person, from the daemon to the screens: the profile, the people, the saved views, the two
 * tables' sort, and the progress through onboarding.
 *
 * The two sides were made to look alike, so this is one word for another in most places. What is
 * here is the few places they are not:
 *
 *   - The store sorts by `{ k, dir }` with `dir` 1 or -1; the wire says `{ key, direction }` with
 *     "asc" or "desc".
 *   - The store's filter chips are `{ k, v }`; the wire's are `{ key, value }`.
 *   - A folded lane is a key `"<project>:<swimlane>:<lane>"` of one map for every project in the
 *     store, and `"<swimlane>:<lane>"` in a project's own list on the wire.
 *   - The store's time zone is `tz`; the wire's is `timeZone`.
 */

/** The sort the store keeps for a wire order. A table that was never sorted keeps `fallback`. */
export function toSortSpec(order: WireSortOrder | null | undefined, fallback: SortSpec): SortSpec {
  if (!order) return { ...fallback };
  return { k: order.key, dir: order.direction === "desc" ? -1 : 1 };
}

export const toWireSort = (spec: SortSpec): WireSortOrder => ({
  key: spec.k,
  direction: spec.dir < 0 ? "desc" : "asc",
});

export const toStoredFilters = (filters: readonly WireFilter[]): Filter[] =>
  filters.map((filter) => ({ k: filter.key, v: filter.value }));

export const toWireFilters = (filters: readonly Filter[]): WireFilter[] =>
  filters.map((filter) => ({ key: filter.k, value: filter.v }));

/** The keys of `S.laneCollapsed` that belong to a project, without the project. */
export function toWireLanes(pid: string, folded: Readonly<Record<string, boolean>>): string[] {
  const prefix = `${pid}:`;
  return Object.entries(folded)
    .filter(([key, on]) => on && key.startsWith(prefix))
    .map(([key]) => key.slice(prefix.length))
    .sort();
}

/** The key of one folded lane in `S.laneCollapsed`, from the wire's `"<swimlane>:<lane>"`. */
export const toLaneKey = (pid: string, lane: string): string => `${pid}:${lane}`;

/** A saved view as the store keeps it, with the daemon's id so the view in use can be told apart from a namesake. */
export const toStoredView = (view: WireSavedView): SavedView => ({
  id: view.id,
  name: view.name,
  f: toStoredFilters(view.filters),
  swim: view.swimlane,
});

/** The store's own "All cards": every card, no grouping. It is not on the daemon and has no id. */
export const ALL_CARDS = "All cards";

const sameName = (a: string, b: string): boolean =>
  a.trim().toLowerCase() === b.trim().toLowerCase();

/**
 * True for a view that shows what a project shows before anything is chosen: named "All cards", with
 * no filters and no grouping. Using it and using none are the same thing, so the daemon is told
 * none, and a daemon that has such a view of its own (a fixture may) does not make the client's
 * own "All cards" in use look like a choice.
 */
export const isAllCards = (view: Pick<SavedView, "name" | "f" | "swim">): boolean =>
  sameName(view.name, ALL_CARDS) && view.f.length === 0 && view.swim === "none";

/**
 * A project's saved views as the menu lists them: the daemon's, and the client's own "All cards"
 * first unless the daemon has a view of that name, so the menu never lists the name twice.
 */
export function toViewList(views: readonly WireSavedView[]): SavedView[] {
  const stored = views.map(toStoredView);
  if (stored.some((view) => sameName(view.name, ALL_CARDS))) return stored;
  return [{ name: ALL_CARDS, f: [], swim: "none" }, ...stored];
}

/** The profile fields the screens read. The tailnet, the node, and the devices are not the daemon's yet. */
export type StoredProfile = Pick<Profile, "id" | "name" | "email" | "tz" | "initials">;

export const toStoredProfile = (profile: WireProfile): StoredProfile => ({
  id: profile.id,
  name: profile.name,
  email: profile.email,
  tz: profile.timeZone,
  initials: profile.initials,
});

export const toPerson = (user: WireUser): Person => ({ id: user.id, name: user.name });

/** What the onboarding screens read, from what the daemon saved. */
export interface OnboardingState {
  /** True while the first-launch screens still show. */
  onboarding: boolean;
  /** The screen they resume at. */
  obStep: number;
  /** True when the tour on Home should run: onboarding is over and the tour was neither finished nor skipped. */
  tourPending: boolean;
}

export function toOnboardingState(progress: WireProgress): OnboardingState {
  const onboarding = progress.onboarding.status === "pending";
  return {
    onboarding,
    obStep: progress.onboarding.step,
    tourPending: !onboarding && progress.tutorial.status === "pending",
  };
}
