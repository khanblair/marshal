import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { Tour } from "./Tour";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const DESKTOP = { vw: 1440, vh: 900 };
const PHONE = { vw: 390, vh: 844 };
const MEASURE_INTERVAL_MS = 250;
const SETTLE_MS = 30;
const ENDED_MESSAGE = "Tour finished. Replay it from your profile menu.";

type Box = [left: number, top: number, width: number, height: number];

function place(el: Element, [left, top, width, height]: Box): void {
  el.getBoundingClientRect = () =>
    ({
      left,
      top,
      width,
      height,
      right: left + width,
      bottom: top + height,
      x: left,
      y: top,
    }) as DOMRect;
}

/** An app root at the page's corner, so a target's box is its own position. */
function addRoot(size = DESKTOP): HTMLElement {
  const root = document.createElement("div");
  root.setAttribute("data-app-root", "1");
  place(root, [0, 0, size.vw, size.vh]);
  document.body.append(root);
  return root;
}

function addTarget(
  root: Element,
  name: string,
  box: [number, number, number, number],
): HTMLElement {
  const el = document.createElement("div");
  el.setAttribute("data-tour", name);
  place(el, box);
  root.append(el);
  return el;
}

const popover = (): HTMLElement => screen.getByRole("dialog");
const cutout = (container: HTMLElement): HTMLElement | null =>
  container.querySelector<HTMLElement>(".shadow-\\[0_0_0_9999px_var\\(--color-tour-dim\\)\\]");
const press = (key: string): boolean => fireEvent.keyDown(window, { key });

beforeEach(() => {
  vi.useFakeTimers();
  M._scale = 1;
  M.set({ ...DESKTOP, tour: { step: 0 }, toasts: [], onboarding: false });
});

afterEach(() => {
  cleanup();
  document.body.replaceChildren();
  vi.useRealTimers();
});

describe("Tour popover", () => {
  it("is a modal dialog with the step's label, title, and text", () => {
    render(() => <Tour />);
    expect(popover()).toHaveAttribute("aria-modal", "true");
    expect(popover()).toHaveAccessibleName("Summary and charts");
    expect(popover()).toHaveAccessibleDescription(/The tiles and charts show what is working/);
    expect(screen.getByText("Step 1 of 9")).toBeInTheDocument();
  });

  it("has Skip tour and Next, and Back only after the first step", () => {
    render(() => <Tour />);
    expect(screen.getByRole("button", { name: "Skip tour" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Next" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Back" })).toBeNull();
    M.set({ tour: { step: 1 } });
    expect(screen.getByRole("button", { name: "Back" })).toBeInTheDocument();
  });

  it("follows the step in the store, and the last step says Finish tour", () => {
    render(() => <Tour />);
    M.set({ tour: { step: 8 } });
    expect(screen.getByText("Step 9 of 9")).toBeInTheDocument();
    expect(popover()).toHaveAccessibleName("Your profile");
    expect(screen.getByRole("button", { name: "Finish tour" })).toBeInTheDocument();
  });

  it("does not move focus when it opens", () => {
    render(() => <Tour />);
    vi.advanceTimersByTime(SETTLE_MS * 2);
    expect(screen.getByRole("button", { name: "Next" })).not.toHaveFocus();
  });
});

describe("Tour buttons", () => {
  it("Next goes to the next step and takes focus", () => {
    render(() => <Tour />);
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(M.S.tour).toEqual({ step: 1 });
    expect(screen.getByText("Step 2 of 9")).toBeInTheDocument();
    vi.advanceTimersByTime(SETTLE_MS);
    expect(screen.getByRole("button", { name: "Next" })).toHaveFocus();
  });

  it("Back goes to the step before, and never before the first", () => {
    M.set({ tour: { step: 2 } });
    render(() => <Tour />);
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(M.S.tour).toEqual({ step: 1 });
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(M.S.tour).toEqual({ step: 0 });
    expect(screen.queryByRole("button", { name: "Back" })).toBeNull();
  });

  it("Skip tour ends the tour with a toast", () => {
    render(() => <Tour />);
    fireEvent.click(screen.getByRole("button", { name: "Skip tour" }));
    expect(M.S.tour).toBeNull();
    expect(M.S.toasts.map((toast) => toast.msg)).toEqual([ENDED_MESSAGE]);
  });

  it("Finish tour on the last step ends it the same way", () => {
    M.set({ tour: { step: 8 } });
    render(() => <Tour />);
    fireEvent.click(screen.getByRole("button", { name: "Finish tour" }));
    expect(M.S.tour).toBeNull();
    expect(M.S.toasts.map((toast) => toast.msg)).toEqual([ENDED_MESSAGE]);
  });
});

describe("Tour keys", () => {
  it("Escape ends the tour and is not passed on", () => {
    render(() => <Tour />);
    const cancelled = !press("Escape");
    expect(cancelled).toBe(true);
    expect(M.S.tour).toBeNull();
    expect(M.S.toasts.map((toast) => toast.msg)).toEqual([ENDED_MESSAGE]);
  });

  it("Enter goes to the next step", () => {
    render(() => <Tour />);
    const cancelled = !press("Enter");
    expect(cancelled).toBe(true);
    expect(M.S.tour).toEqual({ step: 1 });
  });

  it("Enter on the last step ends the tour", () => {
    M.set({ tour: { step: 8 } });
    render(() => <Tour />);
    press("Enter");
    expect(M.S.tour).toBeNull();
  });

  it("Enter does nothing while typing in a field", () => {
    render(() => <Tour />);
    const input = document.body.appendChild(document.createElement("input"));
    input.focus();
    press("Enter");
    expect(M.S.tour).toEqual({ step: 0 });
  });

  it("other keys and keys after the tour ended are ignored", () => {
    render(() => <Tour />);
    press("a");
    expect(M.S.tour).toEqual({ step: 0 });
    M.set({ tour: null });
    press("Escape");
    expect(M.S.toasts).toHaveLength(0);
  });

  it("stops listening when the tour is closed", () => {
    const { unmount } = render(() => <Tour />);
    unmount();
    press("Escape");
    expect(M.S.tour).toEqual({ step: 0 });
  });
});

describe("Tour target", () => {
  it("dims everything when the target is not on the page", () => {
    const { container } = render(() => <Tour />);
    expect(cutout(container)).toBeNull();
    expect(container.querySelector(".bg-tour-dim")).toBeInTheDocument();
  });

  it("cuts a box 4 px around the target", () => {
    const root = addRoot();
    addTarget(root, "tiles", [100, 60, 600, 200]);
    const { container } = render(() => <Tour />);
    expect(container.querySelector(".bg-tour-dim")).toBeNull();
    expect(cutout(container)).toHaveStyle({
      left: "96px",
      top: "56px",
      width: "608px",
      height: "208px",
    });
  });

  it("puts the popover under a wide target", () => {
    const root = addRoot();
    addTarget(root, "tiles", [100, 60, 600, 200]);
    render(() => <Tour />);
    expect(popover()).toHaveStyle({ left: "100px", top: "272px", width: "320px" });
  });

  it("puts the popover beside a narrow target", () => {
    const root = addRoot();
    addTarget(root, "tiles", [16, 300, 200, 40]);
    render(() => <Tour />);
    expect(popover()).toHaveStyle({ left: "228px", top: "300px" });
  });

  it("centers the popover without a target", () => {
    render(() => <Tour />);
    expect(popover()).toHaveStyle({ left: "560px", top: "360px" });
  });
});

describe("Tour target tracking", () => {
  it("measures again every 250 ms and follows a target that moves", () => {
    const root = addRoot();
    const target = addTarget(root, "tiles", [100, 60, 600, 200]);
    const { container } = render(() => <Tour />);
    place(target, [300, 160, 600, 200]);
    vi.advanceTimersByTime(MEASURE_INTERVAL_MS - 1);
    expect(cutout(container)).toHaveStyle({ left: "96px" });
    vi.advanceTimersByTime(1);
    expect(cutout(container)).toHaveStyle({ left: "296px", top: "156px" });
  });

  it("ignores a move of 1 px or less", () => {
    const root = addRoot();
    const target = addTarget(root, "tiles", [100, 60, 600, 200]);
    const { container } = render(() => <Tour />);
    place(target, [100.5, 60, 600, 200]);
    vi.advanceTimersByTime(MEASURE_INTERVAL_MS);
    expect(cutout(container)).toHaveStyle({ left: "96px" });
  });

  it("goes back to the full dim when the target disappears", () => {
    const root = addRoot();
    const target = addTarget(root, "tiles", [100, 60, 600, 200]);
    const { container } = render(() => <Tour />);
    target.remove();
    vi.advanceTimersByTime(MEASURE_INTERVAL_MS);
    expect(cutout(container)).toBeNull();
    expect(container.querySelector(".bg-tour-dim")).toBeInTheDocument();
  });

  it("finds the next step's target right after Next", () => {
    const root = addRoot();
    addTarget(root, "tiles", [100, 60, 600, 200]);
    addTarget(root, "needs", [100, 300, 600, 100]);
    const { container } = render(() => <Tour />);
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    vi.advanceTimersByTime(SETTLE_MS);
    expect(cutout(container)).toHaveStyle({ top: "296px", height: "108px" });
  });

  it("divides by the device frame's scale", () => {
    M._scale = 0.5;
    const root = addRoot();
    addTarget(root, "tiles", [50, 30, 300, 100]);
    const { container } = render(() => <Tour />);
    expect(cutout(container)).toHaveStyle({
      left: "96px",
      top: "56px",
      width: "608px",
      height: "208px",
    });
  });

  it("uses the phone name of a target when the desktop one is missing", () => {
    const root = addRoot(PHONE);
    addTarget(root, "projects-phone", [10, 8, 200, 32]);
    M.set({ tour: { step: 3 } });
    const { container } = render(() => <Tour />);
    expect(cutout(container)).toHaveStyle({ left: "6px", top: "4px" });
  });
});

describe("Tour on a phone", () => {
  beforeEach(() => M.set(PHONE));

  it("is a sheet along the bottom edge", () => {
    render(() => <Tour />);
    expect(popover()).toHaveClass("bottom-0", "inset-x-0", "rounded-t-xl");
    expect(popover()).not.toHaveClass("border");
    expect(popover().style.left).toBe("");
  });

  it("is a bordered card next to the target on a larger screen", () => {
    M.set(DESKTOP);
    render(() => <Tour />);
    expect(popover()).toHaveClass("rounded-lg", "border");
    expect(popover()).not.toHaveClass("bottom-0");
  });
});

describe("Tour click layer", () => {
  it("swallows clicks so nothing underneath sees them", () => {
    const { container } = render(() => <Tour />);
    const outside = vi.fn();
    document.addEventListener("click", outside);
    fireEvent.click(container.querySelector(".pointer-events-auto") as HTMLElement);
    document.removeEventListener("click", outside);
    expect(outside).not.toHaveBeenCalled();
  });

  it("lets the tour's own layer catch pointer events over a click-through wrapper", () => {
    const { container } = render(() => <Tour />);
    expect(container.firstElementChild).toHaveClass("pointer-events-none");
    expect(popover()).toHaveClass("pointer-events-auto");
  });
});

describe("Tour without a tour in the store", () => {
  it("does not throw when the tour ends while it is shown", () => {
    render(() => <Tour />);
    expect(() => M.set({ tour: null })).not.toThrow();
    vi.advanceTimersByTime(MEASURE_INTERVAL_MS);
  });
});
