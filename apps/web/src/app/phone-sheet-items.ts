/** The rows of the phone's Go to and More sheets, as the design's `renderVals` builds them. */
import type { IconNameInput } from "@marshal/ui";
import { M } from "~/mock";
import { closeMenu, newProjectDraft, openPalette } from "./shell-actions";
import { isProject } from "./shell-layout";

export interface SheetItem {
  icon: IconNameInput;
  label: string;
  run: () => void;
  current?: boolean;
  /** Cards that need you, shown as a badge. */
  badge?: number;
  hint?: string;
}

const openNewProject = (): void => M.set({ menu: null, newProject: newProjectDraft() });

/** Runs `run` after closing the sheet. */
const closing =
  (run: () => void): (() => void) =>
  () => {
    closeMenu();
    run();
  };

/** The rows of the project picker: Home, every project, and New project. */
export function pickerItems(): SheetItem[] {
  const S = M.S;
  return [
    {
      icon: "house",
      label: "Home",
      current: S.route.page === "home",
      badge: M.needs().length,
      run: closing(() => M.go("home")),
    },
    ...S.projects.map((project) => ({
      icon: "folder-git-2" as const,
      label: project.name,
      current: isProject() && S.route.pid === project.id,
      badge: M.needs(project.id).length,
      run: closing(() => M.go("project", project.id)),
    })),
    { icon: "folder-plus", label: "New project", run: openNewProject },
  ];
}

const openView =
  (view: "list" | "timeline" | "calendar"): (() => void) =>
  () =>
    M.go("project", M.S.route.pid, view);

function newCard(): void {
  if (!isProject()) M.go("project", M.S.route.pid, "board");
  M.newCard();
}

/** The rows of the More sheet. */
export function moreItems(): SheetItem[] {
  return [
    {
      icon: "search",
      label: "Search",
      hint: "Actions, cards, settings",
      run: closing(openPalette),
    },
    { icon: "list", label: "List view", run: closing(openView("list")) },
    { icon: "gantt-chart", label: "Timeline view", run: closing(openView("timeline")) },
    { icon: "calendar", label: "Calendar view", run: closing(openView("calendar")) },
    { icon: "plus", label: "New card", run: closing(newCard) },
    { icon: "folder-plus", label: "New project", run: openNewProject },
    { icon: "settings", label: "Settings", run: closing(() => M.go("settings")) },
    { icon: "map", label: "Replay tour", run: () => M.startTour() },
  ];
}
