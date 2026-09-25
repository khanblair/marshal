import { fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type Card, M } from "~/mock";
import { TimelineView } from "./TimelineView";
import { rangeLabel } from "./timeline-geometry";
import { timelineCards } from "./timeline-model";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const seed: Card[] = structuredClone(JSON.parse(JSON.stringify(M.S.cards)));
const DESKTOP_W = 1440;
const PHONE_W = 390;
const DAY_PX = 40;
const PRESS_X = 300;

const live = (id: number): Card => {
  const c = M.card(id);
  if (!c) throw new Error(`no card ${id}`);
  return c;
};
const rows = () => [...document.querySelectorAll<HTMLElement>("[data-card]")];
const barOf = (id: number): HTMLElement => {
  const bar = document.querySelector<HTMLElement>(`[data-card="${id}"] + div [role="button"]`);
  if (!bar) throw new Error(`no bar for ${id}`);
  return bar;
};
const move = (x: number) => window.dispatchEvent(new PointerEvent("pointermove", { clientX: x }));
const release = () => window.dispatchEvent(new PointerEvent("pointerup"));
const press = (id: number) =>
  fireEvent.pointerDown(barOf(id), { clientX: PRESS_X, button: 0, pointerId: 1 });
const dragBy = (id: number, days: number) => {
  press(id);
  move(PRESS_X + days * DAY_PX);
  release();
};

beforeEach(() => {
  vi.useFakeTimers();
  M.S.cards = structuredClone(seed);
  M.clearFilters();
  M.go("project", "api", "timeline");
  M.set({ vw: DESKTOP_W, openId: null, dialog: null, toasts: [] });
});
afterEach(() => {
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe("TimelineView on desktop", () => {
  it("shows one row per planned card of the project, sorted by start day", () => {
    render(() => <TimelineView />);
    const expected = timelineCards(M.filtered("api")).map((c) => String(c.id));
    expect(rows().map((r) => r.dataset.card)).toEqual(expected);
    expect(expected.length).toBeGreaterThan(5);
  });

  it("shows the date range, the day headings, and the legend", () => {
    render(() => <TimelineView />);
    expect(screen.getByText(rangeLabel(M.T0, M.D))).toBeInTheDocument();
    expect(document.querySelectorAll("[title]").length).toBeGreaterThan(32);
    expect(screen.getByText("Depends on")).toBeInTheDocument();
    expect(screen.getByText("Broken dependency")).toBeInTheDocument();
    expect(screen.getByText("Drag a bar to change its planned dates.")).toBeInTheDocument();
  });

  it("opens the card from its row", () => {
    render(() => <TimelineView />);
    fireEvent.click(rows()[0] as HTMLElement);
    expect(M.S.openId).toBe(Number(rows()[0]?.dataset.card));
  });

  it("labels each row for screen readers", () => {
    render(() => <TimelineView />);
    const first = rows()[0] as HTMLElement;
    expect(first.getAttribute("aria-label")).toBe(M.deco(live(Number(first.dataset.card))).aria);
  });

  it("gives each bar a tooltip with what it depends on", () => {
    render(() => <TimelineView />);
    expect(barOf(46).title).toMatch(/^#46 .*\. Depends on #39/);
    expect(barOf(39).title).toMatch(/Blocks #46/);
    expect(barOf(46).tabIndex).toBe(-1);
  });

  it("draws a line for each dependency, red and dashed only when broken", () => {
    render(() => <TimelineView />);
    const paths = () => [...document.querySelectorAll("svg path[stroke-dasharray]")];
    expect(paths().length).toBeGreaterThan(0);
    expect(paths().filter((p) => p.getAttribute("stroke-dasharray") === "4 3")).toHaveLength(0);
    live(45).s = -3;
    expect(paths().filter((p) => p.getAttribute("stroke-dasharray") === "4 3")).toHaveLength(1);
  });

  it("shows only the cards the filters leave", () => {
    M.addFilter("status", "working");
    render(() => <TimelineView />);
    const all = timelineCards(M.filtered("api"));
    expect(rows()).toHaveLength(all.length);
    expect(all.every((c) => c.state === "working")).toBe(true);
  });

  it("keeps the arrow-key rows for the shell while mounted and clears them on unmount", () => {
    const view = render(() => <TimelineView />);
    expect(M.nav?.owner).toBe("timeline");
    expect(M.nav?.rows).toEqual(rows().map((r) => Number(r.dataset.card)));
    view.unmount();
    expect(M.nav).toBeNull();
  });
});

describe("dragging a bar", () => {
  const free = () => {
    const cards = timelineCards(M.filtered("api"));
    const c = cards.find((x) => !x.deps.length && !cards.some((o) => o.deps.includes(x.id)));
    if (!c) throw new Error("no free card");
    return c;
  };

  it("opens the card when the bar is pressed and released without moving", () => {
    render(() => <TimelineView />);
    press(46);
    release();
    expect(M.S.openId).toBe(46);
  });

  it("ignores a press with another mouse button", () => {
    render(() => <TimelineView />);
    fireEvent.pointerDown(barOf(46), { clientX: PRESS_X, button: 2 });
    release();
    expect(M.S.openId).toBeNull();
  });

  it("moves the bar with the pointer in whole days and lifts it with a shadow", () => {
    render(() => <TimelineView />);
    const c = free();
    const left = barOf(c.id).style.left;
    press(c.id);
    move(PRESS_X + 2 * DAY_PX + 5);
    expect(parseInt(barOf(c.id).style.left, 10)).toBe(parseInt(left, 10) + 2 * DAY_PX);
    expect(barOf(c.id)).toHaveClass("shadow-drag");
    release();
    expect(barOf(c.id)).not.toHaveClass("shadow-drag");
  });

  it("moves the planned dates on drop and says so", () => {
    render(() => <TimelineView />);
    const c = free();
    const { s, e } = c;
    dragBy(c.id, 2);
    expect([live(c.id).s, live(c.id).e]).toEqual([(s ?? 0) + 2, (e ?? 0) + 2]);
    expect(M.S.toasts.map((t) => t.msg)).toContain("Dates updated");
    expect(M.S.openId).toBeNull();
  });

  it("changes nothing when the pointer returns to the start", () => {
    render(() => <TimelineView />);
    const c = free();
    const { s } = c;
    press(c.id);
    move(PRESS_X + 3 * DAY_PX);
    move(PRESS_X);
    release();
    expect(live(c.id).s).toBe(s);
    expect(M.S.dialog).toBeNull();
    expect(M.S.toasts).toHaveLength(0);
  });

  it("asks before a move that breaks a dependency, and moves on Move anyway", () => {
    render(() => <TimelineView />);
    const { s } = live(46);
    dragBy(46, -3);
    expect(M.S.dialog?.title).toBe("Break a dependency");
    expect(M.S.dialog?.action).toBe("Move anyway");
    expect(M.S.dialog?.message).toContain("#46 would start before #39 finishes");
    expect(live(46).s).toBe(s);
    M.S.dialog?.run();
    expect(live(46).s).toBe((s ?? 0) - 3);
  });

  it("stops following the pointer when the browser cancels it", () => {
    render(() => <TimelineView />);
    const c = free();
    const { s } = c;
    press(c.id);
    move(PRESS_X + 2 * DAY_PX);
    window.dispatchEvent(new PointerEvent("pointercancel"));
    move(PRESS_X + 5 * DAY_PX);
    release();
    expect(live(c.id).s).toBe(s);
    expect(barOf(c.id)).not.toHaveClass("shadow-drag");
    expect(M.S.openId).toBeNull();
  });

  it("removes its window listeners when the view goes away mid drag", () => {
    const remove = vi.spyOn(window, "removeEventListener");
    const view = render(() => <TimelineView />);
    press(46);
    view.unmount();
    const types = remove.mock.calls.map(([type]) => type);
    expect(types).toEqual(expect.arrayContaining(["pointermove", "pointerup", "pointercancel"]));
    remove.mockRestore();
  });
});

describe("TimelineView on a phone", () => {
  beforeEach(() => {
    M.set({ vw: PHONE_W });
  });

  it("lists the cards under their start day instead of drawing a grid", () => {
    render(() => <TimelineView />);
    expect(
      screen.getByText("Cards by planned start day. Open a card to change its dates."),
    ).toBeInTheDocument();
    expect(document.getElementById("tl-arrow")).toBeNull();
    expect(screen.getAllByRole("heading", { level: 3 }).length).toBeGreaterThan(1);
  });

  it("says One day or Until for each card", () => {
    render(() => <TimelineView />);
    expect(screen.getAllByText(/^Until /).length).toBeGreaterThan(0);
  });

  it("lists what a card waits for, and opens that card", () => {
    render(() => <TimelineView />);
    const chip = screen.getByRole("button", { name: /^#39 / });
    expect(chip).toBeInTheDocument();
    fireEvent.click(chip);
    expect(M.S.openId).toBe(39);
  });

  it("marks a card it waits for red when that card ends too late", () => {
    live(45).s = -3;
    render(() => <TimelineView />);
    const late = screen.getAllByRole("button", { name: /^#36 / })[0] as HTMLElement;
    expect(late.style.color).toBe(M.tone("danger", "text"));
  });

  it("opens a card from its title", () => {
    render(() => <TimelineView />);
    const first = timelineCards(M.filtered("api"))[0] as Card;
    fireEvent.click(screen.getByText(first.title));
    expect(M.S.openId).toBe(first.id);
  });
});
