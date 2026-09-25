import { fireEvent, render, screen, within } from "@solidjs/testing-library";
import { describe, expect, it, vi } from "vitest";
import { type Card, M } from "~/mock";
import { BoardView } from "./BoardView";
import { DONE_LIMIT } from "./board-model";
import { cardIds, column, PHONE_PX, reset, useBoardTestStore } from "./board-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const FIRST_CLONE_ID = 400;

function addDoneCards(count: number): void {
  const template = M.card(33);
  if (!template) throw new Error("seed card 33 missing");
  for (let i = 0; i < count; i++) {
    M.S.cards.push({
      ...JSON.parse(JSON.stringify(template)),
      id: FIRST_CLONE_ID + i,
      title: `Old card ${i}`,
      upd: template.upd - (i + 1),
    } satisfies Card);
  }
}

useBoardTestStore();

describe("BoardView columns", () => {
  it("draws the seven columns with their counts, in order", () => {
    render(() => <BoardView />);
    const names = screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent);
    expect(names).toEqual([
      "Backlog",
      "Planning",
      "Working",
      "Needs you",
      "In review",
      "Ready to merge",
      "Done",
    ]);
    const counts = Array.from(document.querySelectorAll("h2 + span")).map((el) =>
      el.getAttribute("aria-label"),
    );
    expect(counts).toEqual([
      "1 cards",
      "1 cards",
      "2 cards",
      "2 cards",
      "2 cards",
      "2 cards",
      "1 cards",
    ]);
    expect(column("working")).toHaveAttribute("aria-label", "Working, 2 cards");
    expect(column("backlog")).toHaveAttribute("aria-label", "Backlog, 1 cards");
  });

  it("puts each card in its column, newest first, and merging cards in Ready to merge", () => {
    render(() => <BoardView />);
    expect(cardIds(column("working"))).toEqual([42, 41]);
    expect(cardIds(column("ready")).sort()).toEqual([35, 36]);
    expect(cardIds(column("done"))).toEqual([33]);
  });

  it("has no tabs on desktop and fills its parent", () => {
    const { container } = render(() => <BoardView />);
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(container.firstElementChild).toHaveClass("absolute", "inset-0", "flex-col", "bg-canvas");
  });

  it("opens a card on click", () => {
    render(() => <BoardView />);
    fireEvent.click(within(column("needs")).getByRole("button", { name: /^#43 / }));
    expect(M.S.openId).toBe(43);
  });

  it("lights the border of the column a dragged card is over and says where to drop", () => {
    render(() => <BoardView />);
    expect(column("review")).toHaveClass("border-transparent");
    M.set({ dragId: 41, dropCol: "review" });
    expect(column("review")).toHaveClass("border-border-strong");
    expect(column("review")).not.toHaveClass("border-transparent");
    M.S.cards = M.S.cards.filter((c) => M.colOf(c.state) !== "planning");
    expect(within(column("planning")).getByText("Drop here")).toBeInTheDocument();
  });

  it("says No cards in an empty column", () => {
    M.S.cards = M.S.cards.filter((c) => M.colOf(c.state) !== "planning");
    render(() => <BoardView />);
    expect(within(column("planning")).getByText("No cards")).toBeInTheDocument();
  });
});

describe("BoardView keyboard navigation model", () => {
  it("publishes the card grid on M.nav and clears it on unmount", () => {
    const view = render(() => <BoardView />);
    expect(M.nav?.owner).toBe("board");
    expect(M.nav?.grid).toHaveLength(7);
    expect(M.nav?.grid?.[2]).toEqual([42, 41]);
    view.unmount();
    expect(M.nav).toBeNull();
  });

  it("does not clear a model another view took over", () => {
    const view = render(() => <BoardView />);
    M.nav = { owner: "list", rows: [] };
    view.unmount();
    expect(M.nav?.owner).toBe("list");
  });
});

describe("BoardView filters and empty states", () => {
  it("shows only the filtered cards and counts them", () => {
    M.addFilter("status", "needs");
    render(() => <BoardView />);
    expect(cardIds(column("needs"))).toEqual([43, 44]);
    expect(cardIds(column("working"))).toEqual([]);
    expect(within(column("working")).getByText("No cards")).toBeInTheDocument();
  });

  it("says nothing matches a search and clears it", () => {
    M.S.query.api = "zzzz";
    render(() => <BoardView />);
    expect(screen.getByText('No cards match "zzzz".')).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(M.S.query.api).toBe("");
    expect(screen.queryByText(/No cards match/)).not.toBeInTheDocument();
  });

  it("says filters match nothing when there is no search text", () => {
    M.addFilter("role", "Tester");
    M.addFilter("agent", "Built-in agent");
    M.addFilter("label", "nothing-has-this-label");
    render(() => <BoardView />);
    expect(screen.getByText("No cards match these filters.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(M.S.filters.api).toEqual([]);
  });

  it("offers a New card button on an empty board", () => {
    M.S.cards = M.S.cards.filter((c) => c.p !== "api");
    render(() => <BoardView />);
    expect(screen.getByText("This board has no cards yet.")).toBeInTheDocument();
    expect(document.querySelector("section[data-col]")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "New card" }));
    expect(M.S.newCard).toMatchObject({ template: "Blank", start: false });
  });
});

describe("BoardView Done limit", () => {
  it("shows the newest 20 done cards and a Show all button", () => {
    addDoneCards(DONE_LIMIT + 5);
    render(() => <BoardView />);
    expect(cardIds(column("done"))).toHaveLength(DONE_LIMIT);
    fireEvent.click(
      within(column("done")).getByRole("button", { name: `Show all ${DONE_LIMIT + 6}` }),
    );
    expect(M.S.showAllDone.api).toBe(true);
    expect(cardIds(column("done"))).toHaveLength(DONE_LIMIT + 6);
    expect(
      within(column("done")).queryByRole("button", { name: /Show all/ }),
    ).not.toBeInTheDocument();
  });

  it("has no Show all button up to the limit", () => {
    addDoneCards(DONE_LIMIT - 1);
    render(() => <BoardView />);
    expect(screen.queryByRole("button", { name: /Show all/ })).not.toBeInTheDocument();
  });
});

describe("BoardView on a phone", () => {
  beforeEach(() => reset("api", PHONE_PX));

  it("shows tabs for the seven columns, with Working selected", () => {
    render(() => <BoardView />);
    const tabs = within(screen.getByRole("tablist", { name: "Columns" })).getAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual([
      "Backlog1",
      "Planning1",
      "Working2",
      "Needs you2",
      "In review2",
      "Ready to merge2",
      "Done1",
    ]);
    expect(tabs.map((t) => t.getAttribute("aria-selected"))).toEqual([
      "false",
      "false",
      "true",
      "false",
      "false",
      "false",
      "false",
    ]);
    expect(document.querySelectorAll("section[data-col]")).toHaveLength(1);
    expect(column("working")).toHaveClass("w-full", "min-h-60");
  });

  it("does not draw column headers or lanes", () => {
    M.S.swim.api = "role";
    render(() => <BoardView />);
    expect(screen.queryByRole("heading", { level: 2 })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { expanded: true })).not.toBeInTheDocument();
  });

  it("switches column from a tab", () => {
    render(() => <BoardView />);
    fireEvent.click(screen.getByRole("tab", { name: /^Needs you/ }));
    expect(M.S.mobileCol).toBe("needs");
    expect(screen.getByRole("tab", { name: /^Needs you/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(document.querySelectorAll("section[data-col]")).toHaveLength(1);
    expect(cardIds(column("needs"))).toEqual([43, 44]);
  });

  const scroller = (): HTMLElement => {
    const el = column("working").parentElement?.parentElement?.parentElement;
    if (!el) throw new Error("no scroll area");
    return el;
  };
  const touch = (x: number, y: number) => ({
    touches: [{ clientX: x, clientY: y }],
    changedTouches: [{ clientX: x, clientY: y }],
  });

  it("swipes to the next and previous column, and stops at the ends", () => {
    render(() => <BoardView />);
    const area = scroller();
    fireEvent.touchStart(area, touch(300, 200));
    fireEvent.touchEnd(area, touch(150, 210));
    expect(M.S.mobileCol).toBe("needs");
    fireEvent.touchStart(area, touch(100, 200));
    fireEvent.touchEnd(area, touch(250, 190));
    expect(M.S.mobileCol).toBe("working");
    M.S.mobileCol = "backlog";
    fireEvent.touchStart(area, touch(100, 200));
    fireEvent.touchEnd(area, touch(250, 200));
    expect(M.S.mobileCol).toBe("backlog");
  });

  it("ignores a vertical scroll", () => {
    render(() => <BoardView />);
    const area = scroller();
    fireEvent.touchStart(area, touch(200, 100));
    fireEvent.touchEnd(area, touch(120, 400));
    expect(M.S.mobileCol).toBe("working");
  });
});
