import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M, type ViewKey } from "~/mock";
import { toggleColumn } from "./ColumnsMenu";
import { FilterBar } from "./FilterBar";
import { viewDescription } from "./SavedViewsMenu";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const DESKTOP_PX = 1440;
const PHONE_PX = 390;

function showProject(view: ViewKey, width = DESKTOP_PX, pid = "api"): void {
  M.setViewport(width, 900);
  M.set({ openId: null, menu: null, toasts: [] });
  M.S.filters[pid] = [];
  M.S.query[pid] = "";
  M.S.savedView[pid] = pid === "mobile" ? "By package" : "All cards";
  M.go("project", pid, view);
}

const openMenu = (button: string) => {
  fireEvent.click(screen.getByRole("button", { name: button }));
  return screen.getByRole("menu");
};

beforeEach(() => showProject("board"));
afterEach(cleanup);

describe("Add filter menu", () => {
  it("toggles open and closed with aria-expanded", () => {
    render(() => <FilterBar />);
    const button = screen.getByRole("button", { name: "Add filter" });
    expect(button).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(button);
    expect(button).toHaveAttribute("aria-expanded", "true");
    expect(M.S.menu).toBe("filter");
    fireEvent.click(button);
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("lists the groups with a count on every option, zero included", () => {
    render(() => <FilterBar />);
    const menu = openMenu("Add filter");
    for (const heading of ["Status", "Role", "Agent", "Label"]) {
      expect(within(menu).getByText(heading)).toBeInTheDocument();
    }
    const working = within(menu).getByRole("menuitem", { name: /Working/ });
    expect(working.textContent).toContain(
      String(M.cardsOf("api").filter((c) => M.colOf(c.state) === "working").length),
    );
    expect(within(menu).getAllByRole("menuitem").length).toBeGreaterThan(10);
  });

  it("adds a filter and closes the menu", () => {
    render(() => <FilterBar />);
    fireEvent.click(within(openMenu("Add filter")).getByRole("menuitem", { name: /Needs you/ }));
    expect(M.S.filters.api).toEqual([{ k: "status", v: "needs" }]);
    expect(screen.queryByRole("menu")).toBeNull();
    expect(screen.getByRole("button", { name: "Remove filter Status needs" })).toBeInTheDocument();
  });

  it("has a Package group for a monorepo project", () => {
    showProject("board", DESKTOP_PX, "mobile");
    render(() => <FilterBar />);
    const menu = openMenu("Add filter");
    expect(within(menu).getByText("Package")).toBeInTheDocument();
    expect(
      within(menu).getByRole("menuitem", { name: /packages\/api-client/ }),
    ).toBeInTheDocument();
  });

  it("stays open on an outside press on desktop, as in the design", () => {
    render(() => <FilterBar />);
    openMenu("Add filter");
    fireEvent.pointerDown(document.body);
    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  it("is a bottom sheet on phones that closes on an outside press", () => {
    showProject("board", PHONE_PX);
    render(() => <FilterBar />);
    const menu = openMenu("Add filter");
    expect(menu).toHaveClass("fixed", "bottom-0");
    fireEvent.pointerDown(document.body);
    expect(M.S.menu).toBeNull();
  });
});

describe("Saved views menu", () => {
  it("shows the current view's name, or Unsaved view", () => {
    render(() => <FilterBar />);
    expect(screen.getByRole("button", { name: "All cards" })).toBeInTheDocument();
    M.S.savedView.api = null;
    expect(screen.getByRole("button", { name: "Unsaved view" })).toBeInTheDocument();
  });

  it("lists the views as radios with the current one checked and a description", () => {
    render(() => <FilterBar />);
    const menu = openMenu("All cards");
    const radios = within(menu).getAllByRole("menuitemradio");
    expect(radios.map((r) => r.getAttribute("aria-checked"))).toEqual(["true", "false", "false"]);
    expect(within(menu).getByText("1 filter")).toBeInTheDocument();
    expect(within(menu).getByText("By role")).toBeInTheDocument();
  });

  it("applies a view and closes the menu", () => {
    render(() => <FilterBar />);
    fireEvent.click(within(openMenu("All cards")).getByRole("menuitemradio", { name: /Needs me/ }));
    expect(M.S.savedView.api).toBe("Needs me");
    expect(M.S.filters.api).toEqual([{ k: "status", v: "needs" }]);
    expect(M.S.menu).toBeNull();
  });

  it("saves the current filters under the typed name", () => {
    M.addFilter("label", "auth");
    render(() => <FilterBar />);
    const menu = openMenu("Unsaved view");
    const name = within(menu).getByRole("textbox", { name: "Name this view" });
    fireEvent.input(name, { target: { value: "  Auth work  " } });
    fireEvent.submit(name.closest("form") as HTMLFormElement);
    expect(M.S.savedView.api).toBe("Auth work");
    expect(M.S.savedViews.api?.some((v) => v.name === "Auth work")).toBe(true);
    expect(M.S.menu).toBeNull();
  });

  it("does not save without a name", () => {
    render(() => <FilterBar />);
    const before = M.S.savedViews.api?.length;
    const menu = openMenu("All cards");
    fireEvent.submit(
      within(menu)
        .getByRole("textbox", { name: "Name this view" })
        .closest("form") as HTMLFormElement,
    );
    expect(M.S.savedViews.api).toHaveLength(before ?? 0);
  });

  it("describes swimlanes, filter counts, and plain views", () => {
    const view = { name: "v", f: [], swim: "none" as const };
    expect(viewDescription(view)).toBe("");
    expect(viewDescription({ ...view, swim: "agent" })).toBe("By agent");
    expect(viewDescription({ ...view, f: [{ k: "role", v: "Worker" }] })).toBe("1 filter");
    expect(
      viewDescription({
        ...view,
        f: [
          { k: "role", v: "Worker" },
          { k: "agent", v: "Codex" },
        ],
      }),
    ).toBe("2 filters");
  });
});

describe("Columns menu", () => {
  beforeEach(() => {
    showProject("list");
    M.S.listCols.pkg = false;
    M.S.listCols.cost = true;
  });

  it("lists the twelve columns as checkboxes in the design's order", () => {
    render(() => <FilterBar />);
    const menu = openMenu("Columns");
    const boxes = within(menu).getAllByRole("checkbox");
    expect(boxes).toHaveLength(12);
    expect(within(menu).getByText("Thinking mode")).toBeInTheDocument();
    expect(within(menu).getByRole("checkbox", { name: "Package" })).not.toBeChecked();
    expect(within(menu).getByRole("checkbox", { name: "Cost" })).toBeChecked();
  });

  it("toggles a column in the store", () => {
    render(() => <FilterBar />);
    const menu = openMenu("Columns");
    fireEvent.click(within(menu).getByRole("checkbox", { name: "Package" }));
    expect(M.S.listCols.pkg).toBe(true);
    fireEvent.click(within(menu).getByRole("checkbox", { name: "Cost" }));
    expect(M.S.listCols.cost).toBe(false);
  });

  it("keeps the Title column on", () => {
    render(() => <FilterBar />);
    const title = within(openMenu("Columns")).getByRole("checkbox", { name: "Title" });
    fireEvent.click(title);
    expect(M.S.listCols.title).toBe(true);
    expect(title).toBeChecked();
  });

  it("puts a box back the way the store has it for the required column", () => {
    const box = document.createElement("input");
    box.type = "checkbox";
    box.checked = false;
    toggleColumn("title", box);
    expect(box.checked).toBe(true);
  });
});
