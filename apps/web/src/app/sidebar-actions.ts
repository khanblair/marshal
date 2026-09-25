/** Actions of the sidebar and the top bar, as the design's `renderVals` defines them. */
import { batch } from "solid-js";
import { M } from "~/mock";
import { newProjectDraft } from "./shell-actions";
import { isTablet } from "./shell-layout";

/** Closes the tablet sidebar overlay. */
export const closeSide = (): void => M.set({ sideOpen: false });

/** The sidebar button: on tablets it opens the overlay, elsewhere it collapses the sidebar. */
export function toggleSidebar(): void {
  if (isTablet()) M.set({ sideOpen: !M.S.sideOpen });
  else M.set({ sidebarCollapsed: !M.S.sidebarCollapsed });
}

export function goHome(): void {
  M.set({ sideOpen: false });
  M.go("home");
}

export function goSettings(): void {
  M.set({ sideOpen: false });
  M.go("settings");
}

export function goProject(id: string): void {
  M.set({ sideOpen: false });
  M.go("project", id);
}

export function openNewProject(): void {
  M.set({ newProject: newProjectDraft(), sideOpen: false });
}

/** Opens Settings on the project section of one project. */
export function openProjectSettings(id: string): void {
  batch(() => {
    M.S.settingsSection = "project";
    M.S.settingsPid = id;
    M.S.sideOpen = false;
    M.go("settings");
  });
}

export function startRename(id: string): void {
  M.set({ renaming: id, menu: null });
}

export function askRemoveProject(id: string): void {
  M.set({
    removeProject: { id, keepBranches: true, keepMemory: true },
    menu: null,
    sideOpen: false,
  });
}

/** The theme button: switches theme, then says which one is on. */
export function toggleTheme(): void {
  M.setTheme(M.S.resolvedTheme === "dark" ? "light" : "dark");
  M.toast(M.S.resolvedTheme === "dark" ? "Dark theme on" : "Light theme on");
}

export function toggleNotices(): void {
  M.set({ noticesOpen: !M.S.noticesOpen, menu: null });
}

/** The header title button on phones opens the project picker. */
export const openPicker = (): void => M.set({ menu: "picker" });
