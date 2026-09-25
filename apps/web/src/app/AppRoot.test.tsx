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

const DETAIL = "unreachable (no answer): Marshal can't reach the daemon. Check that it is running.";
const connectTo = (
  state: NonNullable<typeof M.S.connection>["state"],
  more: Partial<NonNullable<typeof M.S.connection>> = {},
) => {
  M.S.connection = { state, retryAt: null, detail: "", rejection: "", busy: false, ...more };
};

describe("AppRoot and the daemon", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    M.S.connection = undefined;
    M.S.loadError = undefined;
    M.S.ready = true;
  });

  it("draws the app, and no connection screen, while online or with no daemon at all", () => {
    render(() => <AppRoot />);
    expect(appRoot()).toHaveAttribute("data-connection", "online");
    expect(screen.getByRole("banner")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Can't reach the daemon" })).toBeNull();
    connectTo("online");
    expect(appRoot()).toHaveAttribute("data-connection", "online");
    expect(screen.queryByText(/You're offline/)).toBeNull();
  });

  it("shows a skeleton of the app, in its own layout, while the first data loads", () => {
    M.S.ready = false;
    connectTo("starting");
    render(() => <AppRoot />);
    expect(screen.getByRole("status")).toHaveTextContent("Loading Marshal");
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByRole("banner")).toBeNull();
    expect(screen.queryByRole("heading", { name: "Can't reach the daemon" })).toBeNull();
    M.S.ready = true;
    connectTo("online");
    expect(screen.getByRole("banner")).toBeInTheDocument();
    expect(screen.queryByText("Loading Marshal")).toBeNull();
  });

  it("shows Can't reach the daemon, with its technical detail only inside Details, and retries on demand", () => {
    vi.useFakeTimers();
    const retry = vi.spyOn(M, "reconnect").mockImplementation(() => {});
    connectTo("unreachable", { retryAt: Date.now() + 4000, detail: DETAIL });
    M.S.ready = true;
    render(() => <AppRoot />);
    expect(screen.getByRole("heading", { name: "Can't reach the daemon" })).toBeInTheDocument();
    expect(appRoot()).toHaveAttribute("data-connection", "unreachable");
    expect(screen.queryByRole("banner")).toBeNull();
    expect(screen.getByText("Trying again in 4 s")).toBeInTheDocument();
    vi.advanceTimersByTime(1000);
    expect(screen.getByText("Trying again in 3 s")).toBeInTheDocument();
    const detail = screen.getByText(DETAIL);
    expect(detail.closest("details")).not.toBeNull();
    expect(document.body.textContent?.split(DETAIL)).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(retry).toHaveBeenCalledOnce();
  });

  it("goes back to the app by itself when the connection returns", () => {
    connectTo("unreachable", { detail: DETAIL });
    render(() => <AppRoot />);
    expect(screen.getByRole("heading", { name: "Can't reach the daemon" })).toBeInTheDocument();
    connectTo("online");
    expect(screen.queryByRole("heading", { name: "Can't reach the daemon" })).toBeNull();
    expect(screen.getByRole("banner")).toBeInTheDocument();
  });

  it("shows the app with the offline bar on top while it reconnects", () => {
    connectTo("reconnecting");
    render(() => <AppRoot />);
    const bar = screen.getByText(/You're offline/);
    expect(bar.closest("[role=status]")).not.toBeNull();
    expect(screen.getByRole("banner")).toBeInTheDocument();
    expect(appRoot()).toHaveAttribute("data-connection", "reconnecting");
    connectTo("online");
    expect(screen.queryByText(/You're offline/)).toBeNull();
  });

  it("shows sign-in for a device the daemon does not know, and hands over the trimmed token", () => {
    const signIn = vi.spyOn(M, "signIn").mockImplementation(() => {});
    connectTo("unauthorized");
    render(() => <AppRoot />);
    expect(screen.getByRole("heading", { name: "Sign in to Marshal" })).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
    fireEvent.input(screen.getByLabelText("Access token"), { target: { value: "  abc123  " } });
    fireEvent.submit(
      screen.getByRole("button", { name: "Sign in" }).closest("form") as HTMLElement,
    );
    expect(signIn).toHaveBeenCalledWith("abc123");
  });

  it("says why a token was refused, in the daemon's words, under the field", () => {
    const sentence = "Sign in again. This device's token is missing or no longer valid.";
    connectTo("unauthorized", { rejection: sentence });
    render(() => <AppRoot />);
    expect(screen.getByRole("alert")).toHaveTextContent(sentence);
    expect(screen.getByLabelText("Access token")).toHaveAttribute("aria-invalid", "true");
  });

  it("shows a busy sign-in while the token is checked", () => {
    connectTo("unauthorized", { busy: true });
    render(() => <AppRoot />);
    expect(screen.getByRole("button", { name: "Sign in" })).toBeDisabled();
  });

  it("shows a load error, with Try again, when the first data could not be loaded", () => {
    const retry = vi.spyOn(M, "reconnect").mockImplementation(() => {});
    M.S.ready = false;
    M.S.loadError = "Marshal ran into a problem. Try again.";
    connectTo("online");
    render(() => <AppRoot />);
    expect(screen.getByText("Marshal ran into a problem. Try again.")).toBeInTheDocument();
    expect(screen.queryByRole("banner")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(retry).toHaveBeenCalledOnce();
  });
});
