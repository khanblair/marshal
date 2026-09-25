import { batch } from "solid-js";
import { type Column, M, type Project } from "~/mock";

/* Where the buttons on Home lead. */

/** The project with the most of something, the first on a tie, as the design's sort picks it. */
function mostBy(count: (project: Project) => number): Project | undefined {
  return [...M.S.projects].sort((a, b) => count(b) - count(a))[0];
}

/** Cards merged today: merges in the feed, plus done cards updated today that have no merge entry. */
export function mergedTodayCount(): number {
  const merges = M.S.feed.filter((f) => f.kind === "merge" && f.ts >= M.T0);
  const doneWithoutMerge = M.S.cards.filter(
    (c) => c.state === "done" && c.upd >= M.T0 && !merges.some((f) => f.cardId === c.id),
  );
  return merges.length + doneWithoutMerge.length;
}

/** Opens the List of the project with the most cards in that column, filtered to that column. */
export function openStatusList(status: Column): void {
  const best = mostBy(
    (p) => M.S.cards.filter((c) => c.p === p.id && M.colOf(c.state) === status).length,
  );
  if (!best) return;
  batch(() => {
    M.S.filters[best.id] = [{ k: "status", v: status }];
    M.S.savedView[best.id] = null;
  });
  M.go("project", best.id, "list");
}

export function openCostLimits(): void {
  M.S.settingsSection = "limits";
  M.go("settings");
}

/** Opens Settings on the integrations list, where GitHub is connected. */
export function openIntegrations(): void {
  M.S.settingsSection = "integrations";
  M.go("settings");
}

export function viewAllActivity(): void {
  M.set({ allKind: "activity" });
  M.go("all");
}

export function viewAllCi(): void {
  M.set({ allKind: "ci" });
  M.go("all");
}

/** The project with the most awake cards, where the calendar and agents links go by default. */
const busiestProjectId = (): string | undefined => mostBy((p) => M.awake(p.id).length)?.id;

/** Opens the calendar of the current project, or of the busiest one when Home has none. */
export function openCalendar(): void {
  const { pid } = M.S.route;
  M.go("project", pid && M.proj(pid) ? pid : busiestProjectId(), "calendar");
}

export function openAgents(): void {
  M.go("project", busiestProjectId(), "agents");
}
