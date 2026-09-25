import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { PhoneSheet } from "./PhoneSheet";
import { PHONE_PX, resetShell } from "./shell-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => resetShell(PHONE_PX));
afterEach(cleanup);

const rows = () => within(screen.getByRole("dialog")).getAllByRole("button");
const row = (name: string | RegExp) =>
  within(screen.getByRole("dialog")).getByRole("button", { name });

describe("PhoneSheet", () => {
  it("draws nothing while no sheet menu is open, or off phones", () => {
    render(() => <PhoneSheet />);
    expect(screen.queryByRole("dialog")).toBeNull();
    M.set({ menu: "picker" });
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    M.setViewport(1440, 900);
    expect(screen.queryByRole("dialog")).toBeNull();
    M.setViewport(PHONE_PX, 900);
    M.set({ menu: "filter" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("closes from its scrim", () => {
    M.set({ menu: "more" });
    const { container } = render(() => <PhoneSheet />);
    const scrim = container.querySelector<HTMLElement>(".bg-scrim-sheet");
    if (scrim) fireEvent.click(scrim);
    expect(M.S.menu).toBeNull();
  });
});

describe("Go to sheet", () => {
  beforeEach(() => M.set({ menu: "picker" }));

  it("lists Home, every project, and New project", () => {
    render(() => <PhoneSheet />);
    expect(screen.getByRole("dialog", { name: "Go to" })).toHaveAttribute("aria-modal", "true");
    expect(rows().map((r) => r.textContent?.replace(/\d+$/, ""))).toEqual([
      "Home",
      "api-gateway",
      "web-dashboard",
      "mobile-app",
      "New project",
    ]);
  });

  it("shows the needs-you counts and marks the current page", () => {
    M.go("project", "web", "board");
    M.set({ menu: "picker" });
    render(() => <PhoneSheet />);
    expect(row(/^Home/)).toHaveTextContent(String(M.needs().length));
    expect(row(/^web-dashboard/)).toHaveAttribute("aria-current", "page");
    expect(row(/^api-gateway/)).not.toHaveAttribute("aria-current");
  });

  it("goes to a project and closes", () => {
    render(() => <PhoneSheet />);
    fireEvent.click(row(/^mobile-app/));
    expect(M.S.route).toMatchObject({ page: "project", pid: "mobile" });
    expect(M.S.menu).toBeNull();
  });

  it("goes Home and closes", () => {
    M.go("project", "api", "board");
    M.set({ menu: "picker" });
    render(() => <PhoneSheet />);
    fireEvent.click(row(/^Home/));
    expect(M.S.route.page).toBe("home");
    expect(M.S.menu).toBeNull();
  });

  it("New project opens the dialog with an empty draft", () => {
    render(() => <PhoneSheet />);
    fireEvent.click(row("New project"));
    expect(M.S.newProject).toMatchObject({ source: "folder", name: "" });
    expect(M.S.menu).toBeNull();
  });
});

describe("More sheet", () => {
  beforeEach(() => M.set({ menu: "more" }));

  it("lists the eight actions, Search with its hint", () => {
    render(() => <PhoneSheet />);
    expect(screen.getByRole("dialog", { name: "More" })).toBeInTheDocument();
    expect(rows().map((r) => r.textContent)).toEqual([
      "SearchActions, cards, settings",
      "List view",
      "Timeline view",
      "Calendar view",
      "New card",
      "New project",
      "Settings",
      "Replay tour",
    ]);
  });

  it("Search opens the palette", () => {
    render(() => <PhoneSheet />);
    fireEvent.click(row(/^Search/));
    expect(M.S.palette).toBe(true);
    expect(M.S.menu).toBeNull();
  });

  it.each([
    ["List view", "list"],
    ["Timeline view", "timeline"],
    ["Calendar view", "calendar"],
  ] as const)("%s opens that view of the current project", (label, view) => {
    M.go("project", "web", "board");
    M.set({ menu: "more" });
    render(() => <PhoneSheet />);
    fireEvent.click(row(label));
    expect(M.S.route).toMatchObject({ pid: "web", view });
    expect(M.S.menu).toBeNull();
  });

  it("New card goes to the board first when not on a project", () => {
    render(() => <PhoneSheet />);
    fireEvent.click(row("New card"));
    expect(M.S.route).toMatchObject({ page: "project", view: "board" });
    expect(M.S.newCard).not.toBeNull();
  });

  it("New card on a project keeps the view", () => {
    M.go("project", "api", "list");
    M.set({ menu: "more" });
    render(() => <PhoneSheet />);
    fireEvent.click(row("New card"));
    expect(M.S.route.view).toBe("list");
    expect(M.S.newCard).not.toBeNull();
  });

  it("Settings opens Settings, and Replay tour starts the tour", () => {
    render(() => <PhoneSheet />);
    fireEvent.click(row("Settings"));
    expect(M.S.route.page).toBe("settings");
    M.set({ menu: "more" });
    fireEvent.click(row("Replay tour"));
    expect(M.S.tour).toEqual({ step: 0 });
  });

  it("New project opens the dialog", () => {
    render(() => <PhoneSheet />);
    fireEvent.click(row("New project"));
    expect(M.S.newProject).not.toBeNull();
  });
});
