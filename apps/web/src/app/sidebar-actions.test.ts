import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { PHONE_PX, resetShell, TABLET_PX } from "./shell-test-utils";
import {
  askRemoveProject,
  closeSide,
  goHome,
  goProject,
  goSettings,
  openNewProject,
  openPicker,
  openProjectSettings,
  startRename,
  toggleNotices,
  toggleSidebar,
  toggleTheme,
} from "./sidebar-actions";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => resetShell());

describe("sidebar toggle", () => {
  it("collapses and expands on desktop", () => {
    toggleSidebar();
    expect(M.S.sidebarCollapsed).toBe(true);
    toggleSidebar();
    expect(M.S.sidebarCollapsed).toBe(false);
  });

  it("opens and closes the overlay on tablet", () => {
    M.setViewport(TABLET_PX, 900);
    toggleSidebar();
    expect(M.S.sideOpen).toBe(true);
    expect(M.S.sidebarCollapsed).toBe(false);
    toggleSidebar();
    expect(M.S.sideOpen).toBe(false);
  });

  it("closeSide closes the overlay", () => {
    M.set({ sideOpen: true });
    closeSide();
    expect(M.S.sideOpen).toBe(false);
  });
});

describe("going somewhere closes the tablet sidebar", () => {
  it.each([
    ["home", goHome, "home"],
    ["settings", goSettings, "settings"],
  ] as const)("%s", (_name, run, page) => {
    M.set({ sideOpen: true });
    run();
    expect(M.S.sideOpen).toBe(false);
    expect(M.S.route.page).toBe(page);
  });

  it("goProject opens the project", () => {
    M.set({ sideOpen: true });
    goProject("web");
    expect(M.S.sideOpen).toBe(false);
    expect(M.S.route).toMatchObject({ page: "project", pid: "web" });
  });
});

describe("project rows", () => {
  it("startRename swaps the row for the field and closes the menu", () => {
    M.set({ menu: "proj:api" });
    startRename("api");
    expect(M.S.renaming).toBe("api");
    expect(M.S.menu).toBeNull();
  });

  it("openProjectSettings selects the project section", () => {
    M.set({ sideOpen: true });
    openProjectSettings("web");
    expect(M.S).toMatchObject({ settingsSection: "project", settingsPid: "web", sideOpen: false });
    expect(M.S.route.page).toBe("settings");
  });

  it("askRemoveProject opens the dialog keeping branches and memory", () => {
    M.set({ menu: "proj:mobile", sideOpen: true });
    askRemoveProject("mobile");
    expect(M.S.removeProject).toEqual({ id: "mobile", keepBranches: true, keepMemory: true });
    expect(M.S.menu).toBeNull();
    expect(M.S.sideOpen).toBe(false);
  });

  it("openNewProject starts an empty draft", () => {
    M.set({ sideOpen: true });
    openNewProject();
    expect(M.S.newProject).toMatchObject({ source: "folder", path: "", name: "", branch: "" });
    expect(M.S.sideOpen).toBe(false);
  });
});

describe("top bar", () => {
  it("toggleTheme switches theme and says which one is on", () => {
    toggleTheme();
    expect(M.S.resolvedTheme).toBe("dark");
    expect(M.S.toasts.at(-1)?.msg).toBe("Dark theme on");
    toggleTheme();
    expect(M.S.resolvedTheme).toBe("light");
    expect(M.S.toasts.at(-1)?.msg).toBe("Light theme on");
  });

  it("toggleNotices opens the panel and closes menus", () => {
    M.set({ menu: "avatar" });
    toggleNotices();
    expect(M.S.noticesOpen).toBe(true);
    expect(M.S.menu).toBeNull();
    toggleNotices();
    expect(M.S.noticesOpen).toBe(false);
  });

  it("openPicker opens the phone picker", () => {
    M.setViewport(PHONE_PX, 900);
    openPicker();
    expect(M.S.menu).toBe("picker");
  });
});
