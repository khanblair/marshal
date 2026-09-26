import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { createSignal } from "solid-js";
import { VirtualList, type VirtualListHandle } from "./VirtualList";

/** A stand-in for the browser's ResizeObserver, which jsdom does not have: the test says a size. */
class FakeResizeObserver {
  static all: FakeResizeObserver[] = [];
  readonly watched = new Set<Element>();
  constructor(readonly callback: ResizeObserverCallback) {
    FakeResizeObserver.all.push(this);
  }
  observe(el: Element) {
    this.watched.add(el);
  }
  unobserve(el: Element) {
    this.watched.delete(el);
  }
  disconnect() {
    this.watched.clear();
  }
}

/** Tells every observer watching `el` that it is now `height` px tall. */
function resize(el: Element, height: number) {
  for (const one of FakeResizeObserver.all) {
    if (!one.watched.has(el)) continue;
    const entry = { target: el, borderBoxSize: [{ blockSize: height, inlineSize: 0 }] };
    one.callback([entry as unknown as ResizeObserverEntry], one as unknown as ResizeObserver);
  }
}

const VIEWPORT_PX = 200;
const ROW_PX = 40;
const GAP_PX = 10;
const OVERSCAN_PX = 100;

beforeEach(() => {
  FakeResizeObserver.all = [];
  vi.stubGlobal("ResizeObserver", FakeResizeObserver);
  Object.defineProperty(HTMLElement.prototype, "clientHeight", {
    configurable: true,
    get: () => VIEWPORT_PX,
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(HTMLElement.prototype, "clientHeight");
});

const names = (count: number) => Array.from({ length: count }, (_, i) => `Row ${i}`);
const rows = () => screen.queryAllByRole("listitem");
const rowTexts = () => rows().map((row) => row.textContent);
const scroller = () => screen.getByTestId("scroller");
const scrollTo = (top: number) => {
  scroller().scrollTop = top;
  fireEvent.scroll(scroller());
};
const rowNamed = (name: string) => screen.getByText(name).closest("li") as HTMLElement;

interface ListOptions {
  items?: string[];
  estimate?: (item: string) => number;
  handle?: (handle: VirtualListHandle) => void;
}

const show = (options: ListOptions = {}) =>
  render(() => (
    <VirtualList
      data-testid="scroller"
      items={options.items ?? names(1000)}
      itemKey={(item) => item}
      estimateHeight={options.estimate ?? (() => ROW_PX)}
      gap={GAP_PX}
      overscan={OVERSCAN_PX}
      label="Rows"
      handle={options.handle}
      before={<p>Above the rows</p>}
    >
      {(item) => (
        <button type="button" class="block w-full">
          {item()}
        </button>
      )}
    </VirtualList>
  ));

describe("VirtualList", () => {
  it("draws only the rows near the screen, not the thousand it has", () => {
    show();
    // The screen is 0 to 200, and 100 px of overscan below it reaches the row that starts at 250.
    expect(rowTexts()).toEqual(names(6));
    const list = screen.getByRole("list", { name: "Rows" });
    expect(list).toHaveStyle({ height: `${1000 * (ROW_PX + GAP_PX) - GAP_PX}px` });
  });

  it("puts each row where its place in the list says, and says where that is", () => {
    show();
    expect(rowNamed("Row 0")).toHaveStyle({ top: "0px" });
    expect(rowNamed("Row 3")).toHaveStyle({ top: "150px" });
    expect(rowNamed("Row 3")).toHaveAttribute("aria-posinset", "4");
    expect(rowNamed("Row 3")).toHaveAttribute("aria-setsize", "1000");
  });

  it("scrolls with the rows what it is given to show above them", () => {
    show();
    expect(screen.getByText("Above the rows")).toBeInTheDocument();
    expect(scroller()).toContainElement(screen.getByText("Above the rows"));
  });

  it("draws the rows of the part it is scrolled to, and lets go of the rest", () => {
    show();
    scrollTo(5000);
    // 4900 to 5300 touches the rows that start at 4900 (row 98) to 5250 (row 105).
    expect(rowTexts()).toEqual(names(106).slice(98));
    expect(screen.queryByText("Row 0")).toBeNull();
    scrollTo(0);
    expect(rowTexts()).toEqual(names(6));
  });

  it("keeps the same element for a row while the window slides past it", () => {
    show();
    const row = rowNamed("Row 4");
    scrollTo(60);
    expect(rowNamed("Row 4")).toBe(row);
  });

  it("moves the rows below one that turns out taller than it was thought to be", () => {
    show();
    resize(rowNamed("Row 1"), 200);
    expect(rowNamed("Row 2")).toHaveStyle({ top: `${ROW_PX + GAP_PX + 200 + GAP_PX}px` });
    expect(screen.getByRole("list")).toHaveStyle({
      height: `${1000 * (ROW_PX + GAP_PX) - GAP_PX + (200 - ROW_PX)}px`,
    });
    // Fewer rows fit on the screen now.
    expect(rowTexts()).toEqual(names(3));
  });

  it("ignores a measure of no height, which is what a hidden tab reports", () => {
    show();
    resize(rowNamed("Row 1"), 0);
    expect(rowNamed("Row 2")).toHaveStyle({ top: "100px" });
  });

  it("scrolls by as much when a row above the screen grows, so the screen does not move", () => {
    show();
    scrollTo(5000);
    resize(rowNamed("Row 98"), ROW_PX + 120);
    expect(scroller().scrollTop).toBe(5120);
  });

  it("scrolls by as much when a row above the screen is opened, which changes its estimate", () => {
    const [opened, setOpened] = createSignal<readonly string[]>([]);
    show({ estimate: (item) => (opened().includes(item) ? ROW_PX + 60 : ROW_PX) });
    scrollTo(5000);
    setOpened(["Row 98"]);
    expect(scroller().scrollTop).toBe(5060);
  });

  it("does not scroll when the row at the top of the screen, or one below it, changes", () => {
    const [opened, setOpened] = createSignal<readonly string[]>([]);
    show({ estimate: (item) => (opened().includes(item) ? ROW_PX + 60 : ROW_PX) });
    scrollTo(5000);
    resize(rowNamed("Row 100"), ROW_PX + 80);
    setOpened(["Row 101"]);
    expect(scroller().scrollTop).toBe(5000);
  });

  it("does not scroll when a row on the screen grows", () => {
    show();
    scrollTo(5000);
    resize(rowNamed("Row 101"), ROW_PX + 120);
    expect(scroller().scrollTop).toBe(5000);
  });

  it("measures a row again after its estimate changes, since its look did", () => {
    const [open, setOpen] = createSignal(false);
    show({ estimate: () => (open() ? 100 : ROW_PX) });
    setOpen(true);
    resize(rowNamed("Row 1"), 150);
    expect(rowNamed("Row 2")).toHaveStyle({ top: `${100 + GAP_PX + 150 + GAP_PX}px` });
    setOpen(false);
    // Back to the closed look: the 150 px measured for the open row no longer applies.
    expect(rowNamed("Row 2")).toHaveStyle({ top: `${2 * (ROW_PX + GAP_PX)}px` });
    setOpen(true);
    expect(rowNamed("Row 2")).toHaveStyle({ top: `${100 + GAP_PX + 150 + GAP_PX}px` });
  });

  it("keeps the row that has focus drawn when it is scrolled away, and lets go of it once focus leaves", () => {
    show();
    const button = screen.getByRole("button", { name: "Row 1" });
    button.focus();
    scrollTo(5000);
    expect(screen.getByRole("button", { name: "Row 1" })).toBe(button);
    expect(document.activeElement).toBe(button);
    // In document order, so a Tab from it goes on to the rows below it.
    expect(rowTexts()[0]).toBe("Row 1");
    button.blur();
    expect(screen.queryByText("Row 1")).toBeNull();
  });

  it("draws the rows of a new list in the places of the old ones", () => {
    const [items, setItems] = createSignal(names(1000));
    render(() => (
      <VirtualList
        data-testid="scroller"
        items={items()}
        itemKey={(item) => item}
        estimateHeight={() => ROW_PX}
      >
        {(item) => <span>{item()}</span>}
      </VirtualList>
    ));
    setItems(names(1000).map((name) => `New ${name}`));
    expect(rowTexts()[0]).toBe("New Row 0");
  });

  it("copes with a list that gets shorter than the rows drawn", () => {
    const [items, setItems] = createSignal(names(1000));
    render(() => (
      <VirtualList
        data-testid="scroller"
        items={items()}
        itemKey={(item) => item}
        estimateHeight={() => ROW_PX}
        overscan={OVERSCAN_PX}
      >
        {(item) => <span>{item().toUpperCase()}</span>}
      </VirtualList>
    ));
    setItems(names(2));
    expect(rowTexts()).toEqual(["ROW 0", "ROW 1"]);
  });

  it("keeps its full height inside a flex column, where a list of absolutely placed rows would shrink", () => {
    show();
    expect(screen.getByRole("list")).toHaveClass("flex-none");
  });

  it("draws no list at all for no items, but still what goes above it", () => {
    show({ items: [] });
    expect(screen.queryByRole("list")).toBeNull();
    expect(screen.getByText("Above the rows")).toBeInTheDocument();
  });

  it("scrolls a row to the top of the screen through its handle", () => {
    let handle: VirtualListHandle | undefined;
    show({ handle: (given) => (handle = given) });
    handle?.scrollToIndex(300);
    expect(scroller().scrollTop).toBe(300 * (ROW_PX + GAP_PX));
  });

  it("draws rows of any height without a ResizeObserver", () => {
    vi.unstubAllGlobals();
    show();
    expect(rowTexts()).toEqual(names(6));
  });

  it("stops watching a row once it is no longer drawn", () => {
    show();
    const row = rowNamed("Row 0");
    expect(FakeResizeObserver.all.some((one) => one.watched.has(row))).toBe(true);
    scrollTo(5000);
    expect(FakeResizeObserver.all.some((one) => one.watched.has(row))).toBe(false);
  });
});
