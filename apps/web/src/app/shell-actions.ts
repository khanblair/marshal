/** Small shell actions that several parts of the layout share. */
import { batch } from "solid-js";
import { M, type NewProjectDraft } from "~/mock";

export const closeMenu = (): void => M.set({ menu: null });

/** Toggles one named popover menu (`filter`, `views`, `cols`, `avatar`, `proj:<id>`). */
export const toggleMenu = (name: string): void => M.set({ menu: M.S.menu === name ? null : name });

/** Opens the search palette. The palette resets its own query when it opens. */
export const openPalette = (): void => M.set({ palette: true, menu: null });

/** The empty draft the New project dialog starts from. */
export const newProjectDraft = (): NewProjectDraft => ({
  source: "folder",
  path: "",
  url: "",
  name: "",
  branch: "main",
});

/** Opens Settings at one section. */
export function openSettingsSection(section: string): void {
  batch(() => {
    M.set({ settingsSection: section, menu: null });
    M.go("settings");
  });
}
