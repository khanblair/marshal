import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { Toasts } from "./Toasts";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const DESKTOP_PX = 1440;
const PHONE_PX = 390;
const HEIGHT_PX = 900;
const TOAST_MS = 4000;
const ACTION_TOAST_MS = 8000;

beforeEach(() => {
  vi.useFakeTimers();
  M.setViewport(DESKTOP_PX, HEIGHT_PX);
  M.set({ toasts: [], announce: "" });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  M.set({ toasts: [], announce: "" });
});

const region = (): HTMLElement =>
  document.querySelector<HTMLElement>('[aria-live="polite"]') as HTMLElement;

describe("Toasts", () => {
  it("shows nothing but two empty live regions when there are no toasts", () => {
    render(() => <Toasts />);
    expect(screen.queryAllByRole("status")).toHaveLength(0);
    expect(document.querySelectorAll('[aria-live="polite"]')).toHaveLength(2);
  });

  it("shows a toast on ink with a dismiss button, and removes it after four seconds", () => {
    render(() => <Toasts />);
    M.toast("Card started");
    const toast = screen.getByRole("status");
    expect(toast).toHaveTextContent("Card started");
    expect(toast).toHaveClass("bg-ink", "text-on-ink");
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeInTheDocument();
    vi.advanceTimersByTime(TOAST_MS);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("dismisses a toast from its X", () => {
    render(() => <Toasts />);
    M.toast("Light theme on");
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("runs the action and dismisses the toast; a toast with an action stays eight seconds", () => {
    const undo = vi.fn();
    render(() => <Toasts />);
    M.toast("Chat archived", { label: "Undo", run: undo });
    vi.advanceTimersByTime(TOAST_MS);
    expect(screen.getByRole("status")).toHaveTextContent("Chat archived");
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(undo).toHaveBeenCalledOnce();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("removes an action toast after eight seconds", () => {
    render(() => <Toasts />);
    M.toast("Chat archived", { label: "Undo", run: () => {} });
    vi.advanceTimersByTime(ACTION_TOAST_MS);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("keeps the three newest toasts", () => {
    render(() => <Toasts />);
    for (const msg of ["one", "two", "three", "four"]) M.toast(msg);
    expect(screen.getAllByRole("status").map((t) => t.textContent)).toEqual([
      expect.stringContaining("two"),
      expect.stringContaining("three"),
      expect.stringContaining("four"),
    ]);
  });

  it("sits at the bottom right on desktop and above the bottom navigation on a phone", () => {
    render(() => <Toasts />);
    expect(region()).toHaveClass("bottom-4", "right-4", "left-4", "z-toast");
    cleanup();
    M.setViewport(PHONE_PX, HEIGHT_PX);
    render(() => <Toasts />);
    expect(region()).toHaveClass("bottom-[calc(72px+env(safe-area-inset-bottom))]");
  });

  it("announces state changes in a visually hidden live region", () => {
    render(() => <Toasts />);
    M.set({ announce: "#43 moved to Review" });
    const announcer = screen.getByText("#43 moved to Review");
    expect(announcer).toHaveAttribute("aria-live", "polite");
    expect(announcer).toHaveClass("absolute", "w-px", "h-px", "overflow-hidden");
  });
});
