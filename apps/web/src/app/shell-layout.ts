/**
 * Layout facts the shell reads from the store, as the design's `renderVals` computes
 * them. They are plain functions over `M.S`, so JSX and memos that call them track it.
 */
import { M, type Project, type ViewKey } from "~/mock";

/** Narrower viewports use the phone layout. */
const PHONE_MAX_PX = 640;
/** Viewports this wide and wider use the desktop layout. */
const DESKTOP_MIN_PX = 1200;
/** The card panel's width limits on desktop. */
export const DETAIL_MIN_PX = 480;
export const DETAIL_MAX_PX = 760;

export const isPhone = (): boolean => M.S.vw < PHONE_MAX_PX;
export const isTablet = (): boolean => M.S.vw >= PHONE_MAX_PX && M.S.vw < DESKTOP_MIN_PX;
export const isDesktop = (): boolean => M.S.vw >= DESKTOP_MIN_PX;
/** Phones and tablets get touch-sized controls. */
export const isTouch = (): boolean => !isDesktop();

export const sizeName = (): "phone" | "tablet" | "desktop" => {
  if (isPhone()) return "phone";
  return isTablet() ? "tablet" : "desktop";
};

/** A project page whose project still exists. */
export const isProject = (): boolean => M.S.route.page === "project" && !!M.proj(M.S.route.pid);

export const isProjectView = (view: ViewKey): boolean => isProject() && M.S.route.view === view;

const NO_PROJECT: Pick<Project, "id" | "name" | "lang"> = { id: "", name: "No projects", lang: "" };

/** The route's project, or the first project, or a stand-in when there are none. */
export const currentProject = (): Pick<Project, "id" | "name" | "lang" | "packages"> =>
  M.proj(M.S.route.pid) || M.S.projects[0] || NO_PROJECT;

/** The tablet sidebar, open over the content. */
export const sideOverlay = (): boolean => isTablet() && !!M.S.sideOpen;

/** Whether the sidebar shows labels: expanded on desktop, or the tablet overlay. */
export const sidebarOpen = (): boolean => (isDesktop() ? !M.S.sidebarCollapsed : sideOverlay());

export const detailOpen = (): boolean => !!M.S.openId && !!M.card(M.S.openId);

/** The card panel's width on desktop, kept inside its limits. */
export const detailWidth = (): number => clampDetailWidth(M.S.detailW);

export const clampDetailWidth = (px: number): number =>
  Math.max(DETAIL_MIN_PX, Math.min(DETAIL_MAX_PX, px));

const DESKTOP_PANES = 3;
const TABLET_PANES = 1;

/** How many split panes fit beside the main view. Phones have none. */
export const maxPanes = (): number => {
  if (isDesktop()) return DESKTOP_PANES;
  return isTablet() ? TABLET_PANES : 0;
};

/** The key shown for the Cmd or Ctrl modifier in hints. */
export const modKey = (): string => (/Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl");

/** The header title for the current page. */
export function pageTitle(): string {
  const page = M.S.route.page;
  if (page === "home") return "Home";
  if (page === "all") return M.S.allKind === "ci" ? "CI health" : "Recent activity";
  if (page === "settings") return "Settings";
  return currentProject().name;
}

/** Cards that need you, across all projects. */
export const totalNeeds = (): number => M.needs().length;
