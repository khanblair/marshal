import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { AvatarMenu } from "./AvatarMenu";
import { PHONE_PX, resetShell } from "./shell-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => resetShell());
afterEach(cleanup);

const button = () => screen.getByRole("button", { name: "Profile and settings" });

describe("AvatarMenu", () => {
  it("shows the initials of the profile name", () => {
    render(() => <AvatarMenu />);
    expect(button()).toHaveTextContent("AO");
    M.S.profile.name = "ada lovelace king";
    expect(button()).toHaveTextContent("AL");
    M.S.profile.name = "Solo";
    expect(button()).toHaveTextContent("S");
  });

  it("opens a popover with the profile and four items, and closes on a second press", () => {
    render(() => <AvatarMenu />);
    expect(button()).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(button());
    expect(button()).toHaveAttribute("aria-expanded", "true");
    const menu = screen.getByRole("menu");
    expect(menu).toHaveClass("absolute", "w-[240px]", "z-[200]");
    expect(menu).toHaveTextContent(M.S.profile.name);
    expect(menu).toHaveTextContent(M.S.profile.tailnet);
    expect(screen.getAllByRole("menuitem").map((i) => i.textContent)).toEqual([
      "Profile",
      "Settings",
      "Keyboard shortcuts",
      "Replay tour",
    ]);
    fireEvent.click(button());
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("closes notices when it opens", () => {
    M.set({ noticesOpen: true });
    render(() => <AvatarMenu />);
    fireEvent.click(button());
    expect(M.S.noticesOpen).toBe(false);
  });

  it.each([
    ["Profile", "profile"],
    ["Settings", "general"],
    ["Keyboard shortcuts", "shortcuts"],
  ])("%s opens Settings on %s", (label, section) => {
    M.set({ menu: "avatar" });
    render(() => <AvatarMenu />);
    fireEvent.click(screen.getByRole("menuitem", { name: label }));
    expect(M.S.route.page).toBe("settings");
    expect(M.S.settingsSection).toBe(section);
    expect(M.S.menu).toBeNull();
  });

  it("Replay tour starts the tour on Home", () => {
    M.set({ menu: "avatar" });
    render(() => <AvatarMenu />);
    fireEvent.click(screen.getByRole("menuitem", { name: "Replay tour" }));
    expect(M.S.tour).toEqual({ step: 0 });
    expect(M.S.route.page).toBe("home");
  });

  it("closes from the clear catcher behind it", () => {
    M.set({ menu: "avatar" });
    const { container } = render(() => <AvatarMenu />);
    const catcher = container.querySelector<HTMLElement>(".z-\\[190\\]");
    expect(catcher).toHaveClass("fixed", "inset-0", "bg-transparent");
    if (catcher) fireEvent.click(catcher);
    expect(M.S.menu).toBeNull();
  });

  it("is a bottom sheet without a dim scrim on a phone", () => {
    M.setViewport(PHONE_PX, 900);
    M.set({ menu: "avatar" });
    const { container } = render(() => <AvatarMenu />);
    expect(screen.getByRole("menu")).toHaveClass("fixed", "bottom-0", "z-sheet");
    expect(screen.getByRole("menu")).not.toHaveClass("w-[240px]");
    expect(container.querySelector(".bg-scrim-sheet")).toBeNull();
  });
});
