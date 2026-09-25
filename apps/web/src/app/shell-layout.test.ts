import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import {
  clampDetailWidth,
  currentProject,
  DETAIL_MAX_PX,
  DETAIL_MIN_PX,
  detailOpen,
  detailWidth,
  isDesktop,
  isPhone,
  isProject,
  isProjectView,
  isTablet,
  isTouch,
  maxPanes,
  modKey,
  pageTitle,
  sidebarOpen,
  sideOverlay,
  sizeName,
  totalNeeds,
} from "./shell-layout";
import { DESKTOP_PX, PHONE_PX, resetShell, TABLET_PX } from "./shell-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => resetShell());

describe("size classes", () => {
  it.each([
    [PHONE_PX, "phone", 0, true],
    [TABLET_PX, "tablet", 1, true],
    [DESKTOP_PX, "desktop", 3, false],
  ] as const)("width %i is %s with %i split panes (touch %s)", (width, name, panes, touch) => {
    M.setViewport(width, 900);
    expect(sizeName()).toBe(name);
    expect(maxPanes()).toBe(panes);
    expect(isTouch()).toBe(touch);
    expect([isPhone(), isTablet(), isDesktop()].filter(Boolean)).toHaveLength(1);
  });

  it("switches exactly at 640 and 1200", () => {
    M.setViewport(639, 900);
    expect(isPhone()).toBe(true);
    M.setViewport(640, 900);
    expect(isTablet()).toBe(true);
    M.setViewport(1199, 900);
    expect(isTablet()).toBe(true);
    M.setViewport(1200, 900);
    expect(isDesktop()).toBe(true);
  });
});

describe("sidebar", () => {
  it("is open on desktop unless collapsed", () => {
    expect(sidebarOpen()).toBe(true);
    M.set({ sidebarCollapsed: true });
    expect(sidebarOpen()).toBe(false);
  });

  it("is open on tablet only as the overlay", () => {
    M.setViewport(TABLET_PX, 900);
    expect(sidebarOpen()).toBe(false);
    expect(sideOverlay()).toBe(false);
    M.set({ sideOpen: true });
    expect(sideOverlay()).toBe(true);
    expect(sidebarOpen()).toBe(true);
  });

  it("ignores sideOpen on desktop", () => {
    M.set({ sideOpen: true });
    expect(sideOverlay()).toBe(false);
  });
});

describe("routes", () => {
  it("knows a project page and its view", () => {
    expect(isProject()).toBe(false);
    M.go("project", "api", "list");
    expect(isProject()).toBe(true);
    expect(isProjectView("list")).toBe(true);
    expect(isProjectView("board")).toBe(false);
  });

  it("is not a project page when the project is gone", () => {
    M.go("project", "nope", "board");
    expect(isProject()).toBe(false);
  });

  it("falls back to the first project, then to a stand-in", () => {
    M.go("project", "nope", "board");
    expect(currentProject().id).toBe(M.S.projects[0]?.id);
    const saved = M.S.projects.splice(0);
    try {
      expect(currentProject().name).toBe("No projects");
    } finally {
      M.S.projects.push(...saved);
    }
  });

  it("titles each page", () => {
    expect(pageTitle()).toBe("Home");
    M.set({ allKind: "ci" });
    M.go("all");
    expect(pageTitle()).toBe("CI health");
    M.set({ allKind: "activity" });
    expect(pageTitle()).toBe("Recent activity");
    M.go("settings");
    expect(pageTitle()).toBe("Settings");
    M.go("project", "api", "board");
    expect(pageTitle()).toBe("api-gateway");
  });
});

describe("card panel", () => {
  it("is open only for a card that exists", () => {
    expect(detailOpen()).toBe(false);
    M.set({ openId: "api#999999" });
    expect(detailOpen()).toBe(false);
    M.openCard("api#41");
    expect(detailOpen()).toBe(true);
  });

  it("keeps its width inside 480 and 760", () => {
    expect(clampDetailWidth(100)).toBe(DETAIL_MIN_PX);
    expect(clampDetailWidth(5000)).toBe(DETAIL_MAX_PX);
    expect(clampDetailWidth(600)).toBe(600);
    M.set({ detailW: 900 });
    expect(detailWidth()).toBe(DETAIL_MAX_PX);
  });
});

describe("shared numbers and text", () => {
  it("counts the cards that need you across projects", () => {
    expect(totalNeeds()).toBe(M.needs().length);
    expect(totalNeeds()).toBeGreaterThan(0);
  });

  it("names the modifier key for the platform", () => {
    expect(["⌘", "Ctrl"]).toContain(modKey());
  });
});
