import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M, type ViewKey } from "~/mock";
import { FilterBar } from "./FilterBar";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const DESKTOP_PX = 1440;
const TABLET_PX = 820;
const PHONE_PX = 390;

function showProject(view: ViewKey, width = DESKTOP_PX): void {
  M.setViewport(width, 900);
  M.set({ openId: null, menu: null, toasts: [] });
  M.S.filters.api = [];
  M.S.query.api = "";
  M.S.swim.api = "none";
  M.S.savedView.api = "All cards";
  M.go("project", "api", view);
}

const search = (): HTMLInputElement => screen.getByRole("textbox", { name: "Filter cards" });

beforeEach(() => showProject("board"));
afterEach(cleanup);

describe("FilterBar visibility", () => {
  it.each(["board", "list", "timeline"] as const)("shows on %s", (view) => {
    showProject(view);
    render(() => <FilterBar />);
    expect(search()).toHaveAttribute("data-search", "1");
  });

  it.each(["chat", "agents", "calendar"] as const)("is hidden on %s", (view) => {
    showProject(view);
    render(() => <FilterBar />);
    expect(screen.queryByRole("textbox", { name: "Filter cards" })).toBeNull();
  });

  it("is hidden outside a project", () => {
    M.go("home");
    render(() => <FilterBar />);
    expect(screen.queryByRole("textbox", { name: "Filter cards" })).toBeNull();
  });

  it("gives way to an open card on phones only", () => {
    showProject("board", PHONE_PX);
    M.set({ openId: "api#41" });
    render(() => <FilterBar />);
    expect(screen.queryByRole("textbox", { name: "Filter cards" })).toBeNull();
    cleanup();
    showProject("board", TABLET_PX);
    M.set({ openId: "api#41" });
    render(() => <FilterBar />);
    expect(search()).toBeInTheDocument();
  });
});

describe("FilterBar search", () => {
  it("writes the text to the project's query and offers Clear filters", () => {
    render(() => <FilterBar />);
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();
    fireEvent.input(search(), { target: { value: "config" } });
    expect(M.S.query.api).toBe("config");
    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(M.S.query.api).toBe("");
    expect(search().value).toBe("");
  });

  it("shows the slash key hint on desktop only, and a narrower field on phones", () => {
    render(() => <FilterBar />);
    expect(screen.getByText("/")).toBeInTheDocument();
    expect(search()).toHaveClass("w-[200px]");
    cleanup();
    showProject("board", PHONE_PX);
    render(() => <FilterBar />);
    expect(screen.queryByText("/")).toBeNull();
    expect(search()).toHaveClass("w-[150px]");
  });

  it("scrolls sideways on phones and wraps elsewhere", () => {
    showProject("board", PHONE_PX);
    const { container } = render(() => <FilterBar />);
    expect(container.firstElementChild).toHaveClass("flex-nowrap", "overflow-x-auto", "px-2");
    cleanup();
    showProject("board");
    const desktop = render(() => <FilterBar />);
    expect(desktop.container.firstElementChild).toHaveClass("flex-wrap", "px-4");
  });
});

describe("FilterBar chips", () => {
  it("shows a chip per filter with the status label and a labelled remove button", () => {
    M.addFilter("status", "needs");
    M.addFilter("label", "auth");
    render(() => <FilterBar />);
    expect(screen.getByText("Needs you")).toBeInTheDocument();
    expect(screen.getByText("auth")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove filter Status needs" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove filter Label auth" })).toBeInTheDocument();
  });

  it("removes a chip and Clear filters removes them all", () => {
    M.addFilter("status", "needs");
    M.addFilter("label", "auth");
    render(() => <FilterBar />);
    fireEvent.click(screen.getByRole("button", { name: "Remove filter Status needs" }));
    expect(M.S.filters.api).toEqual([{ k: "label", v: "auth" }]);
    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(M.S.filters.api).toEqual([]);
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();
  });
});

describe("FilterBar swimlanes and columns", () => {
  it("offers Swimlanes on the board, and changing it unselects the saved view", () => {
    render(() => <FilterBar />);
    const select = screen.getByRole("combobox");
    expect(select).toHaveValue("none");
    fireEvent.change(select, { target: { value: "role" } });
    expect(M.S.swim.api).toBe("role");
    expect(M.S.savedView.api).toBeNull();
    expect(screen.getByText("Swimlanes")).toBeInTheDocument();
  });

  it("ignores a swimlane value it does not know", () => {
    render(() => <FilterBar />);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "none" } });
    expect(M.S.swim.api).toBe("none");
  });

  it("offers Columns only on the List view", () => {
    render(() => <FilterBar />);
    expect(screen.queryByRole("button", { name: "Columns" })).toBeNull();
    cleanup();
    showProject("list");
    render(() => <FilterBar />);
    expect(screen.getByRole("button", { name: "Columns" })).toBeInTheDocument();
    expect(screen.queryByText("Swimlanes")).toBeNull();
  });

  it("hides Swimlanes and Columns on phones", () => {
    showProject("board", PHONE_PX);
    render(() => <FilterBar />);
    expect(screen.queryByText("Swimlanes")).toBeNull();
    cleanup();
    showProject("list", PHONE_PX);
    render(() => <FilterBar />);
    expect(screen.queryByRole("button", { name: "Columns" })).toBeNull();
  });
});
