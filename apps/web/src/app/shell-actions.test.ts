import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import {
  closeMenu,
  newProjectDraft,
  openPalette,
  openSettingsSection,
  toggleMenu,
} from "./shell-actions";
import { resetShell } from "./shell-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => resetShell());

describe("menus", () => {
  it("toggles one named menu on and off", () => {
    toggleMenu("filter");
    expect(M.S.menu).toBe("filter");
    toggleMenu("views");
    expect(M.S.menu).toBe("views");
    toggleMenu("views");
    expect(M.S.menu).toBeNull();
  });

  it("closes whatever menu is open", () => {
    M.set({ menu: "avatar" });
    closeMenu();
    expect(M.S.menu).toBeNull();
  });
});

describe("openPalette", () => {
  it("opens the palette and closes menus", () => {
    M.set({ menu: "avatar" });
    openPalette();
    expect(M.S.palette).toBe(true);
    expect(M.S.menu).toBeNull();
  });
});

describe("openSettingsSection", () => {
  it("goes to Settings on the section", () => {
    M.set({ menu: "avatar" });
    openSettingsSection("shortcuts");
    expect(M.S.route.page).toBe("settings");
    expect(M.S.settingsSection).toBe("shortcuts");
    expect(M.S.menu).toBeNull();
  });
});

describe("newProjectDraft", () => {
  it("starts from a folder with the main branch", () => {
    expect(newProjectDraft()).toEqual({
      source: "folder",
      path: "",
      url: "",
      name: "",
      branch: "main",
    });
  });

  it("gives a fresh object each time", () => {
    expect(newProjectDraft()).not.toBe(newProjectDraft());
  });
});
