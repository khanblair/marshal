import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { AppRoot } from "./AppRoot";
import { DESKTOP_PX, PHONE_PX, resetShell } from "./shell-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

vi.mock("~/onboarding/Onboarding", () => ({ Onboarding: () => <div>onboarding stub</div> }));
vi.mock("~/features/tutorial/Tour", () => ({ Tour: () => <div>tour stub</div> }));

const appRoot = (): HTMLElement => {
  const el = document.querySelector<HTMLElement>("[data-app-root]");
  if (!el) throw new Error("no app root");
  return el;
};
const originalSize = { width: window.innerWidth, height: window.innerHeight };
const resizeTo = (width: number, height = originalSize.height) => {
  window.innerWidth = width;
  window.innerHeight = height;
  window.dispatchEvent(new Event("resize"));
};

beforeEach(() => {
  window.localStorage.clear();
  resetShell();
});
afterEach(() => {
  cleanup();
  resizeTo(originalSize.width, originalSize.height);
  M.set({ onboarding: false, tour: null });
});

describe("AppRoot layout", () => {
  it("fills the window and tags the root with size and touch", () => {
    render(() => <AppRoot />);
    const root = appRoot();
    expect(root).toHaveAttribute("data-app-root", "1");
    expect(root).toHaveAttribute("data-size", "tablet");
    expect(root).toHaveAttribute("data-touch", "1");
    expect(root.parentElement).toHaveClass("fixed", "inset-0", "bg-canvas");
    expect(screen.queryByRole("toolbar", { name: "Prototype controls" })).toBeNull();
  });

  it("draws the sidebar and the main column, but no bottom navigation, on a desktop window", () => {
    render(() => <AppRoot />);
    resizeTo(DESKTOP_PX);
    expect(appRoot()).toHaveAttribute("data-size", "desktop");
    expect(appRoot()).toHaveAttribute("data-touch", "0");
    expect(screen.getByRole("navigation", { name: "Main" })).toBeInTheDocument();
    expect(screen.getByRole("banner")).toBeInTheDocument();
    expect(document.querySelector("[data-tour=views-phone]")).toBeNull();
  });

  it("draws the bottom navigation and no sidebar on a phone window", () => {
    render(() => <AppRoot />);
    resizeTo(PHONE_PX);
    expect(appRoot()).toHaveAttribute("data-size", "phone");
    expect(appRoot()).toHaveAttribute("data-touch", "1");
    expect(document.querySelector("[data-tour=views-phone]")).not.toBeNull();
    expect(screen.getAllByRole("navigation")).toHaveLength(1);
    expect(document.querySelector("[data-tour=projects]")).toBeNull();
  });

  it("waits for the store before drawing the app", () => {
    M.S.ready = false;
    render(() => <AppRoot />);
    expect(screen.queryByRole("banner")).toBeNull();
    M.S.ready = true;
    expect(screen.getByRole("banner")).toBeInTheDocument();
  });
});

describe("AppRoot layers", () => {
  it("installs the global keys", () => {
    render(() => <AppRoot />);
    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    expect(M.S.palette).toBe(true);
  });

  it("draws the onboarding and the tour above the app, never both", () => {
    render(() => <AppRoot />);
    M.set({ tour: { step: 0 } });
    expect(screen.getByText("tour stub").parentElement).toHaveClass(
      "z-tour",
      "pointer-events-none",
    );
    M.set({ onboarding: true });
    expect(screen.getByText("onboarding stub").parentElement).toHaveClass("z-onboarding");
    expect(screen.queryByText("tour stub")).toBeNull();
  });

  it("opens the phone sheet from the header picker", () => {
    render(() => <AppRoot />);
    resizeTo(PHONE_PX);
    fireEvent.click(
      document.querySelector<HTMLElement>("[data-tour=projects-phone]") as HTMLElement,
    );
    expect(screen.getByRole("dialog", { name: "Go to" })).toBeInTheDocument();
  });
});
