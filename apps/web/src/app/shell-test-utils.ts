/** Test helper: puts the shared store into a known state before each shell test. */
import { M } from "~/mock";

export const DESKTOP_PX = 1440;
export const TABLET_PX = 820;
export const PHONE_PX = 390;
const HEIGHT_PX = 900;

const ORIGINAL_NAMES = M.S.projects.map((project) => ({ id: project.id, name: project.name }));

/** Sets the viewport width, closes every layer, and opens Home. */
export function resetShell(width: number = DESKTOP_PX): void {
  M.setViewport(width, HEIGHT_PX);
  for (const { id, name } of ORIGINAL_NAMES) {
    const project = M.proj(id);
    if (project) project.name = name;
  }
  M.setTheme("light");
  M.set({
    menu: null,
    noticesOpen: false,
    sideOpen: false,
    sidebarCollapsed: false,
    renaming: null,
    palette: false,
    dialog: null,
    newCard: null,
    newProject: null,
    removeProject: null,
    onboarding: false,
    tour: null,
    openId: null,
    focusId: null,
    detailExpanded: false,
    split: [],
    toasts: [],
    allKind: "activity",
  });
  M.go("home");
}
