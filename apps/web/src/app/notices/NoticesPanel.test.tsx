import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { NoticesPanel } from "./NoticesPanel";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const seedNotices = JSON.parse(JSON.stringify(M.S.notices));
const DESKTOP_PX = 1440;
const PHONE_PX = 390;
const HEIGHT_PX = 900;

beforeEach(() => {
  M.S.notices = structuredClone(seedNotices);
  M.setViewport(DESKTOP_PX, HEIGHT_PX);
  M.go("home");
  M.set({ noticesOpen: true, openId: null, settingsSection: "general" });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  M.set({ noticesOpen: false });
});

const panel = (): HTMLElement => screen.getByRole("region", { name: "Notices" });
const article = (text: string): HTMLElement => {
  const found = within(panel())
    .getAllByRole("article")
    .find((a) => a.textContent?.includes(text));
  if (!found) throw new Error(`no notice with ${text}`);
  return found;
};

describe("NoticesPanel", () => {
  it("renders nothing while closed", () => {
    M.set({ noticesOpen: false });
    render(() => <NoticesPanel />);
    expect(screen.queryByRole("region", { name: "Notices" })).toBeNull();
  });

  it("is a card at the top right on desktop, with one article per notice", () => {
    render(() => <NoticesPanel />);
    expect(panel()).toHaveClass("right-2", "top-13", "rounded-lg", "bg-surface-raised");
    expect(within(panel()).getByRole("heading", { name: "Notices" })).toBeInTheDocument();
    expect(within(panel()).getAllByRole("article")).toHaveLength(4);
    expect(article("4 cards need you")).toHaveTextContent("Across 3 projects");
    expect(article("Main is failing in mobile-app")).toHaveTextContent(
      "The android workflow failed in LoginFlowTest",
    );
  });

  it("fills the space under the top bar on a phone", () => {
    M.setViewport(PHONE_PX, HEIGHT_PX);
    render(() => <NoticesPanel />);
    expect(panel()).toHaveClass("left-0", "right-0", "top-12", "bottom-0", "bg-canvas");
    expect(panel()).not.toHaveClass("rounded-lg");
  });

  it("closes from the close button and from a click outside", () => {
    const { container } = render(() => <NoticesPanel />);
    fireEvent.click(screen.getByRole("button", { name: "Close notices" }));
    expect(M.S.noticesOpen).toBe(false);
    M.set({ noticesOpen: true });
    fireEvent.click(container.querySelector(".z-\\[99\\]") as HTMLElement);
    expect(M.S.noticesOpen).toBe(false);
  });

  it("lists each waiting card with its reason and actions", () => {
    render(() => <NoticesPanel />);
    const needs = article("4 cards need you");
    expect(within(needs).getByText("#43 Add rate limiting per API key")).toBeInTheDocument();
    expect(within(needs).getByText("Plan ready for review")).toHaveClass(
      "text-status-needs-you-text",
    );
    expect(within(needs).getAllByRole("button", { name: "Open" })).toHaveLength(4);
    expect(within(needs).getByRole("button", { name: "Review plan" })).toHaveClass("bg-ink");
    expect(within(needs).getByRole("button", { name: "Approve" })).toHaveClass("bg-ink");
    expect(within(needs).getAllByRole("button", { name: "Open" })[0]).toHaveClass("bg-surface");
    expect(within(needs).queryByRole("button", { name: "Dismiss notice" })).toBeNull();
  });

  it("approves a card and opens one from its row", () => {
    const approve = vi.spyOn(M, "approve").mockImplementation(() => {});
    render(() => <NoticesPanel />);
    const needs = article("4 cards need you");
    fireEvent.click(within(needs).getByRole("button", { name: "Approve" }));
    expect(approve).toHaveBeenCalledWith("api#44");
    fireEvent.click(within(needs).getByText("#119 Migrate tables to TanStack Table v8"));
    expect(M.S.noticesOpen).toBe(false);
    expect(M.S.openId).toBe("web#119");
  });

  it("shows the sleep countdown and wakes all idle cards from Keep all awake", () => {
    render(() => <NoticesPanel />);
    const sleep = article("are idle and will sleep in");
    expect(sleep).toHaveTextContent(/3 cards are idle and will sleep in \d+:\d\d/);
    expect(within(sleep).getAllByRole("button", { name: "Keep awake" })).toHaveLength(3);
    expect(within(sleep).queryByRole("button", { name: "Dismiss notice" })).toBeNull();
    fireEvent.click(within(sleep).getByRole("button", { name: "Keep all awake" }));
    expect(M.S.notices.some((n) => n.kind === "sleep")).toBe(false);
    expect(within(panel()).getAllByRole("article")).toHaveLength(3);
  });

  it("dismisses a notice from its X, and opens limits from a cost notice", () => {
    render(() => <NoticesPanel />);
    fireEvent.click(
      within(article("Main is failing")).getByRole("button", { name: "Dismiss notice" }),
    );
    expect(within(panel()).queryByText("Main is failing in mobile-app")).toBeNull();
    fireEvent.click(
      within(article("near today's cost limit")).getByRole("button", { name: "Open limits" }),
    );
    expect(M.S.settingsSection).toBe("limits");
    expect(M.S.route.page).toBe("settings");
  });

  it("gives the time a full date as its title", () => {
    render(() => <NoticesPanel />);
    const when = within(article("Main is failing")).getByText("22 min ago");
    expect(when).toHaveAttribute("title");
    expect(when.getAttribute("title")).not.toBe("");
  });

  it("explains when there is nothing to show", () => {
    M.S.notices = [];
    vi.spyOn(M, "needs").mockReturnValue([]);
    render(() => <NoticesPanel />);
    expect(
      screen.getByText("No notices. Sleep warnings, CI failures, and cost warnings show here."),
    ).toBeInTheDocument();
    expect(within(panel()).queryAllByRole("article")).toHaveLength(0);
  });
});
