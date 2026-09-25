import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { CommandPalette } from "./CommandPalette";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const DESKTOP_PX = 1440;
const PHONE_PX = 390;
const HEIGHT_PX = 900;
const FOCUS_MS = 20;
const SEARCH = "Search actions, projects, cards, and settings";

beforeEach(() => {
  vi.useFakeTimers();
  M.setViewport(DESKTOP_PX, HEIGHT_PX);
  M.go("project", "api", "board");
  M.set({ palette: true, menu: null, openId: null });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  M.set({ palette: false });
});

const input = (): HTMLInputElement => screen.getByRole("textbox", { name: SEARCH });
/* Plain DOM queries: a role query with 60 rows is slow in jsdom. */
const options = (): HTMLElement[] => [...document.querySelectorAll<HTMLElement>('[role="option"]')];
const selected = (): HTMLElement | undefined =>
  options().find((o) => o.getAttribute("aria-selected") === "true");
const type = (text: string): void => {
  fireEvent.input(input(), { target: { value: text } });
};

describe("CommandPalette", () => {
  it("renders nothing while closed", () => {
    M.set({ palette: false });
    render(() => <CommandPalette />);
    expect(screen.queryByRole("dialog", { name: "Command palette" })).toBeNull();
  });

  it("opens as a modal dialog with the search field, grouped results, and the first row selected", () => {
    render(() => <CommandPalette />);
    const dialog = screen.getByRole("dialog", { name: "Command palette" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveClass("top-[12%]", "rounded-xl", "max-h-[70%]");
    expect(input()).toHaveAttribute("placeholder", SEARCH);
    const list = within(dialog).getByRole("listbox");
    for (const group of ["Actions", "Projects", "Settings", "Cards that need you"]) {
      expect(within(list).getByText(group)).toBeInTheDocument();
    }
    expect(within(list).getByText("New card")).toBeInTheDocument();
    expect(selected()).toBe(options()[0]);
    expect(options()[0]).toHaveAttribute("data-pi", "0");
  });

  it("fills the screen on a phone and hides hints and key hints", () => {
    M.setViewport(PHONE_PX, HEIGHT_PX);
    render(() => <CommandPalette />);
    expect(screen.getByRole("dialog", { name: "Command palette" })).toHaveClass("inset-0");
    expect(document.querySelectorAll("kbd")).toHaveLength(0);
    expect(screen.queryByText("Go")).toBeNull();
  });

  it("shows key hints and project languages on desktop, writing the modifier for this system", () => {
    render(() => <CommandPalette />);
    const keys = [...document.querySelectorAll("kbd")].map((k) => k.textContent);
    expect(keys).toContain("N");
    expect(keys).toContain("Ctrl");
    expect(keys).not.toContain("⌘");
    expect(screen.getAllByText("Go").length).toBeGreaterThan(0);
  });

  it("focuses the search field after a moment and gives focus back when it closes", () => {
    const opener = document.createElement("button");
    document.body.append(opener);
    opener.focus();
    render(() => <CommandPalette />);
    expect(document.activeElement).toBe(opener);
    vi.advanceTimersByTime(FOCUS_MS);
    expect(document.activeElement).toBe(input());
    M.set({ palette: false });
    expect(document.activeElement).toBe(opener);
    opener.remove();
  });

  it("filters by every word and resets the selection while typing", () => {
    render(() => <CommandPalette />);
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(selected()).toBe(options()[1]);
    type("rate limiting");
    expect(options().map((o) => o.textContent)).toEqual([
      expect.stringContaining("#43 Add rate limiting per API key"),
    ]);
    expect(selected()).toBe(options()[0]);
    expect(screen.getByText("Cards")).toBeInTheDocument();
  });

  it("says when nothing matches", () => {
    render(() => <CommandPalette />);
    type("zzzz");
    expect(options()).toHaveLength(0);
    expect(screen.getByText('No results for "zzzz".')).toBeInTheDocument();
  });

  it("moves the selection with the arrow keys and stops at both ends", () => {
    render(() => <CommandPalette />);
    fireEvent.keyDown(input(), { key: "ArrowUp" });
    expect(selected()).toBe(options()[0]);
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(selected()).toBe(options()[2]);
    fireEvent.keyDown(input(), { key: "ArrowUp" });
    expect(selected()).toBe(options()[1]);
    for (let i = 0; i < options().length + 3; i++) fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(selected()).toBe(options()[options().length - 1]);
    vi.runAllTimers();
  });

  it("selects the row under the mouse", () => {
    render(() => <CommandPalette />);
    fireEvent.mouseMove(options()[3] as HTMLElement);
    expect(selected()).toBe(options()[3]);
    expect(options()[3]).toHaveClass("bg-surface-selected");
    expect(options()[0]).toHaveClass("bg-transparent");
  });

  it("runs the selected command on Enter and closes", () => {
    render(() => <CommandPalette />);
    type("go home");
    fireEvent.keyDown(input(), { key: "Enter" });
    expect(M.S.palette).toBe(false);
    expect(M.S.route.page).toBe("home");
  });

  it("does nothing on Enter when there are no results", () => {
    render(() => <CommandPalette />);
    type("zzzz");
    fireEvent.keyDown(input(), { key: "Enter" });
    expect(M.S.palette).toBe(true);
  });

  it("runs a command that is clicked", () => {
    render(() => <CommandPalette />);
    fireEvent.click(screen.getByText("New card"));
    expect(M.S.palette).toBe(false);
    expect(M.S.newCard).not.toBeNull();
    M.set({ newCard: null });
  });

  it("closes from the Esc button and from a click on the scrim", () => {
    const { container } = render(() => <CommandPalette />);
    fireEvent.click(screen.getByRole("button", { name: "Close search" }));
    expect(M.S.palette).toBe(false);
    M.set({ palette: true });
    fireEvent.click(container.querySelector(".z-\\[399\\]") as HTMLElement);
    expect(M.S.palette).toBe(false);
  });

  it("starts empty every time it opens", () => {
    render(() => <CommandPalette />);
    type("rate");
    M.set({ palette: false });
    M.set({ palette: true });
    expect(input()).toHaveValue("");
    expect(options().length).toBeGreaterThan(4);
  });
});
