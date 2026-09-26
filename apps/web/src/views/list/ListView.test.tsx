import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { type CardKey, cardNumber } from "~/mock/card-key";
import { prototypeCards } from "~/testing/prototype-cards";
import { ListView } from "./ListView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const seed = prototypeCards();
const DEFAULT_COLS = { ...M.S.listCols };
const DESKTOP_PX = 1440;
const TABLET_PX = 820;
const PHONE_PX = 390;

function showProject(pid: string, width = DESKTOP_PX): void {
  M.S.cards = structuredClone(seed);
  M.S.listCols = { ...DEFAULT_COLS };
  M.S.sort.list = { k: "id", dir: -1 };
  M.S.filters = {};
  M.S.query = {};
  M.S.focusId = null;
  M.S.openId = null;
  M.setViewport(width, 900);
  M.go("project", pid, "list");
}

const dataCards = (): string[] =>
  screen
    .getAllByRole("row")
    .slice(1)
    .map((row) => row.getAttribute("data-card") ?? "");
const rowOf = (id: CardKey): HTMLElement => {
  const row = document.querySelector<HTMLElement>(`tr[data-card="${id}"]`);
  if (!row) throw new Error(`no row for #${id}`);
  return row;
};
const headerNames = (): string[] =>
  screen.getAllByRole("columnheader").map((h) => h.textContent ?? "");
const cellsOf = (id: CardKey): string[] =>
  [...rowOf(id).querySelectorAll("td")].map((td) => td.textContent ?? "");

beforeEach(() => showProject("api"));
afterEach(() => {
  cleanup();
  M.S.cards = structuredClone(seed);
});

describe("ListView table", () => {
  it("shows the default columns at desktop width, newest card first", () => {
    render(() => <ListView />);
    expect(headerNames()).toEqual([
      "ID",
      "Title",
      "State",
      "Role",
      "Agent",
      "Model",
      "Branch",
      "CI",
      "Cost",
      "Updated",
    ]);
    expect(screen.getByRole("columnheader", { name: "ID" })).toHaveAttribute(
      "aria-sort",
      "descending",
    );
    expect(screen.getByRole("columnheader", { name: "Title" })).toHaveAttribute(
      "aria-sort",
      "none",
    );
    const ids = dataCards();
    const numbers = ids.map(cardNumber);
    expect(numbers).toEqual([...numbers].sort((a, b) => b - a));
    expect(ids).toContain("api#45");
    expect(screen.getByRole("table")).toHaveStyle({ "min-width": "1100px" });
  });

  it("describes a card in its cells and its accessible name", () => {
    render(() => <ListView />);
    expect(rowOf("api#41")).toHaveAttribute(
      "aria-label",
      "#41 Fix token refresh on login. Working",
    );
    expect(cellsOf("api#41").slice(0, 8)).toEqual([
      "#41",
      "Fix token refresh on login",
      "Working",
      "Worker",
      "Claude Code",
      "claude-sonnet-4-5",
      "marshal/41-fix-token-refresh",
      "Running",
    ]);
    expect(cellsOf("api#41")[8]).toBe("$0.84");
  });

  it("leaves the CI, cost, and branch cells empty when a card has none", () => {
    render(() => <ListView />);
    const [, , , , , , branch, ci, cost] = cellsOf("api#45");
    expect([branch, ci, cost]).toEqual(["", "", ""]);
    expect(within(rowOf("api#45")).getByText("Backlog")).toBeVisible();
  });

  it("right-aligns Cost, and sets the branch in the mono font", () => {
    render(() => <ListView />);
    const cells = rowOf("api#41").querySelectorAll("td");
    expect(cells[8]).toHaveClass("text-right");
    expect(cells[6]).toHaveClass("font-mono", "text-caption", "text-secondary", "max-w-[260px]");
    expect(cells[1]).toHaveClass("font-semibold", "max-w-[360px]");
    expect(cells[0]).toHaveClass("text-muted");
    expect(screen.getByRole("button", { name: "Cost" })).toHaveClass("justify-end");
  });

  it("shows Thinking and Package when the Columns menu turns them on", () => {
    showProject("mobile");
    M.S.listCols.pkg = true;
    M.S.listCols.think = true;
    render(() => <ListView />);
    expect(headerNames()).toContain("Thinking");
    expect(headerNames()).toContain("Package");
    expect(screen.getByRole("table")).toHaveStyle({ "min-width": "1320px" });
    const cells = cellsOf("mobile#209");
    expect(cells[headerNames().indexOf("Package")]).toBe("apps/android");
    expect(cells[headerNames().indexOf("Thinking")]).toBe("High");
    const pkg = rowOf("mobile#209").querySelectorAll("td")[headerNames().indexOf("Package")];
    expect(pkg).toHaveClass("font-mono", "text-caption");
    M.S.listCols.pkg = false;
    expect(headerNames()).not.toContain("Package");
  });

  it("says Not supported for a model without thinking", () => {
    M.S.listCols.think = true;
    render(() => <ListView />);
    expect(cellsOf("api#45")[headerNames().indexOf("Thinking")]).toBe("Not supported");
  });

  it("hides Model, Thinking, Package, and Updated below 1200 px", () => {
    showProject("api", TABLET_PX);
    M.S.listCols.pkg = true;
    M.S.listCols.think = true;
    render(() => <ListView />);
    expect(headerNames()).toEqual([
      "ID",
      "Title",
      "State",
      "Role",
      "Agent",
      "Branch",
      "CI",
      "Cost",
    ]);
    expect(screen.getByRole("table")).toHaveStyle({ "min-width": "880px" });
  });

  it("marks bypass, asleep, and pinned cards with an icon before the title", () => {
    showProject("mobile");
    render(() => <ListView />);
    const titleCell = (id: CardKey) => rowOf(id).querySelectorAll("td")[1] as HTMLElement;
    expect(titleCell("mobile#209").querySelector(".lucide-shield-alert")).not.toBeNull();
    expect(titleCell("mobile#209").querySelector("span[aria-hidden]")).toHaveClass(
      "text-status-danger-solid",
    );
    expect(titleCell("mobile#208").querySelector(".lucide-moon")).not.toBeNull();
    expect(titleCell("mobile#207").querySelector(".lucide-pin")).not.toBeNull();
    expect(titleCell("mobile#210").querySelector("svg")).toBeNull();
    expect(titleCell("mobile#208")).toHaveStyle({ color: "var(--color-text-secondary)" });
    expect(titleCell("mobile#210")).toHaveStyle({ color: "var(--color-text-primary)" });
  });

  it("marks the focused card's row and opens a card on click or Enter", () => {
    render(() => <ListView />);
    M.set({ focusId: "api#43" });
    expect(rowOf("api#43")).toHaveClass("bg-surface-selected");
    expect(rowOf("api#41")).not.toHaveClass("bg-surface-selected");
    fireEvent.click(rowOf("api#44"));
    expect(M.S.openId).toBe("api#44");
    M.set({ openId: null });
    fireEvent.keyDown(rowOf("api#41"), { key: "Enter" });
    expect(M.S.openId).toBe("api#41");
    expect(rowOf("api#41")).toHaveAttribute("tabindex", "0");
  });
});

describe("ListView sorting", () => {
  it("sorts by a header and flips on a second click", () => {
    render(() => <ListView />);
    fireEvent.click(screen.getByRole("button", { name: "Title" }));
    expect(M.S.sort.list).toEqual({ k: "title", dir: 1 });
    expect(screen.getByRole("columnheader", { name: "Title" })).toHaveAttribute(
      "aria-sort",
      "ascending",
    );
    expect(screen.getByRole("columnheader", { name: "ID" })).toHaveAttribute("aria-sort", "none");
    const titles = screen
      .getAllByRole("row")
      .slice(1)
      .map((r) => r.querySelectorAll("td")[1]?.textContent?.toLowerCase() ?? "");
    expect(titles).toEqual([...titles].sort());
    fireEvent.click(screen.getByRole("button", { name: "Title" }));
    expect(screen.getByRole("columnheader", { name: "Title" })).toHaveAttribute(
      "aria-sort",
      "descending",
    );
  });

  it("keeps a row element when the order changes", () => {
    render(() => <ListView />);
    const before = rowOf("api#41");
    fireEvent.click(screen.getByRole("button", { name: "Cost" }));
    expect(rowOf("api#41")).toBe(before);
  });
});

describe("ListView filters", () => {
  it("shows only the cards that match a filter", () => {
    M.addFilter("role", "Tester");
    render(() => <ListView />);
    expect(dataCards()).toEqual(["api#42"]);
    expect(screen.queryByText(/No cards match/)).toBeNull();
  });

  it("keeps the headers and says so when nothing matches, and clears on the button", () => {
    M.addFilter("role", "Tester");
    M.addFilter("status", "done");
    render(() => <ListView />);
    expect(screen.getByText("No cards match these filters.")).toBeVisible();
    expect(screen.getAllByRole("columnheader")).toHaveLength(10);
    expect(dataCards()).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(M.S.filters.api).toEqual([]);
    expect(screen.queryByText(/No cards match/)).toBeNull();
    expect(dataCards().length).toBeGreaterThan(5);
  });

  it("quotes the search text in the notice", () => {
    M.S.query.api = "zzz";
    render(() => <ListView />);
    expect(screen.getByText('No cards match "zzz".')).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(M.S.query.api).toBe("");
  });
});

describe("ListView phone list", () => {
  beforeEach(() => showProject("api", PHONE_PX));

  it("shows a stacked row per card instead of a table", () => {
    render(() => <ListView />);
    expect(screen.queryByRole("table")).toBeNull();
    const row = screen.getAllByRole("listitem")[0] as HTMLElement;
    expect(row).toHaveTextContent("#");
    const button = within(row).getByRole("button");
    expect(button).toHaveAttribute("data-card");
    expect(button).toHaveClass("border-l-3", "border-0");
  });

  it("shows CI and cost only when the card has them, and colors the edge from the state", () => {
    render(() => <ListView />);
    const button = (id: CardKey) =>
      document.querySelector<HTMLElement>(`button[data-card="${id}"]`) as HTMLElement;
    expect(button("api#41")).toHaveTextContent("Running");
    expect(button("api#41")).toHaveTextContent("$0.84");
    expect(button("api#41")).toHaveStyle({
      "border-left-color": "var(--color-status-working-solid)",
    });
    expect(button("api#45")).not.toHaveTextContent("$");
    expect(button("api#45")).toHaveStyle({ "border-left-color": "var(--color-border)" });
    fireEvent.click(button("api#41"));
    expect(M.S.openId).toBe("api#41");
  });

  it("shows the notice when nothing matches", () => {
    M.S.query.api = "zzz";
    render(() => <ListView />);
    expect(screen.getByText('No cards match "zzz".')).toBeVisible();
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  });

  it("switches to the table when the width grows", () => {
    render(() => <ListView />);
    M.setViewport(TABLET_PX, 1180);
    expect(screen.getByRole("table")).toBeVisible();
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  });
});

describe("ListView keyboard navigation", () => {
  it("publishes the row order to the shell, and clears it on unmount", () => {
    const { unmount } = render(() => <ListView />);
    expect(M.nav?.owner).toBe("list");
    expect(M.nav?.rows).toEqual(dataCards());
    fireEvent.click(screen.getByRole("button", { name: "ID" }));
    expect(M.nav?.rows).toEqual(dataCards());
    unmount();
    expect(M.nav).toBeNull();
  });

  it("leaves another view's navigation alone on unmount", () => {
    const { unmount } = render(() => <ListView />);
    M.nav = { owner: "agents", rows: ["api#1"] };
    unmount();
    expect(M.nav).toEqual({ owner: "agents", rows: ["api#1"] });
    M.nav = null;
  });
});
