import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { PHONE_PX, resetShell, TABLET_PX } from "./shell-test-utils";
import { TopBar } from "./TopBar";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => resetShell());
afterEach(cleanup);

const noticeTotal = (): number => (M.needs().length > 0 ? 1 : 0) + M.S.notices.length;

describe("TopBar (desktop)", () => {
  it("shows the page title and, on a project, its language", () => {
    M.go("project", "api", "board");
    render(() => <TopBar />);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("api-gateway");
    expect(screen.getByText(M.proj("api")?.lang ?? "")).toBeInTheDocument();
  });

  it("shows no language on Home", () => {
    render(() => <TopBar />);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Home");
    expect(screen.getByRole("banner").querySelector("h1 + span")).toBeNull();
  });

  it("has a search box with the shortcut, and it opens the palette", () => {
    render(() => <TopBar />);
    const search = screen.getByRole("button", { name: "Search" });
    expect(search).toHaveAttribute("title", "Search or run a command");
    expect(search).toHaveAttribute("data-tour", "search");
    expect(search).toHaveClass("w-[220px]", "justify-start");
    expect(within(search).getByText("Search")).toBeInTheDocument();
    expect(within(search).getByText("K")).toBeInTheDocument();
    fireEvent.click(search);
    expect(M.S.palette).toBe(true);
  });

  it("switches theme from its button, named for the theme it switches to", () => {
    render(() => <TopBar />);
    fireEvent.click(screen.getByRole("button", { name: "Switch to dark theme" }));
    expect(M.S.resolvedTheme).toBe("dark");
    expect(screen.getByRole("button", { name: "Switch to light theme" })).toHaveAttribute(
      "title",
      "Switch to light theme",
    );
    expect(M.S.toasts.at(-1)?.msg).toBe("Dark theme on");
  });

  it("counts the notices and toggles the panel", () => {
    render(() => <TopBar />);
    const bell = screen.getByRole("button", { name: `Notices, ${noticeTotal()}` });
    expect(bell).toHaveAttribute("data-tour", "notices");
    expect(bell).toHaveAttribute("title", "Notices");
    expect(bell).toHaveAttribute("aria-expanded", "false");
    expect(bell).toHaveTextContent(String(noticeTotal()));
    fireEvent.click(bell);
    expect(M.S.noticesOpen).toBe(true);
    expect(bell).toHaveAttribute("aria-expanded", "true");
    expect(bell).toHaveClass("bg-surface-selected");
  });

  it("shows no count when there are no notices", () => {
    const needs = vi.spyOn(M, "needs").mockReturnValue([]);
    const saved = M.S.notices.splice(0);
    try {
      render(() => <TopBar />);
      const bell = screen.getByRole("button", { name: "Notices, 0" });
      expect(bell.querySelector(".bg-ink")).toBeNull();
    } finally {
      M.S.notices.push(...saved);
      needs.mockRestore();
    }
  });

  it("has the avatar button with the profile initials", () => {
    render(() => <TopBar />);
    const avatar = screen.getByRole("button", { name: "Profile and settings" });
    expect(avatar).toHaveAttribute("data-tour", "avatar");
    expect(avatar).toHaveTextContent("AO");
  });
});

describe("TopBar (tablet)", () => {
  it("shrinks the search box to an icon", () => {
    M.setViewport(TABLET_PX, 900);
    render(() => <TopBar />);
    const search = screen.getByRole("button", { name: "Search" });
    expect(search).toHaveClass("w-10", "justify-center");
    expect(search).not.toHaveTextContent("Search");
    expect(screen.getByRole("banner")).toHaveClass("px-4");
  });
});

describe("TopBar (phone)", () => {
  beforeEach(() => M.setViewport(PHONE_PX, 900));

  it("turns the title into the project picker button", () => {
    render(() => <TopBar />);
    const picker = screen.getByRole("button", { name: /Home/ });
    expect(picker).toHaveAttribute("data-tour", "projects-phone");
    expect(picker).toHaveAttribute("aria-haspopup", "dialog");
    expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
    expect(screen.queryByRole("button", { name: "Search" })).toBeNull();
    expect(screen.getByRole("banner")).toHaveClass("px-2");
    fireEvent.click(picker);
    expect(M.S.menu).toBe("picker");
  });

  it("shows the needs-you count next to the title away from Home", () => {
    render(() => <TopBar />);
    expect(screen.getByRole("button", { name: /Home/ })).not.toHaveTextContent(/\d/);
    M.go("project", "api", "board");
    expect(screen.getByRole("button", { name: /api-gateway/ })).toHaveTextContent(
      String(M.needs().length),
    );
  });
});
