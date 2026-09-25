import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { HomeView } from "./HomeView";
import {
  type HomeSnapshot,
  homeSnapshot,
  PHONE_WIDTH_PX,
  resetHome,
  TABLET_WIDTH_PX,
} from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const snapshot: HomeSnapshot = homeSnapshot();
/** Rendering all of Home in jsdom is slow, and role queries over its buttons more so. */
const SLOW_TEST_MS = 30_000;

beforeEach(() => {
  vi.useFakeTimers();
  resetHome(snapshot);
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

const section = (name: string) => screen.getByRole("region", { name });
const heading = (name: string) => screen.getByRole("heading", { name, level: 2 });
const setNeedsCardsTo = (state: "working" | "review") => {
  for (const card of M.S.cards.filter((c) => c.state === "needs")) card.state = state;
};

describe("HomeView layout", { timeout: SLOW_TEST_MS }, () => {
  it("has a heading for every block and keeps the tour anchors", () => {
    const { container } = render(() => <HomeView />);
    for (const name of [
      "How it's going",
      "Needs you",
      "Recent activity",
      "Coming up today",
      "Agents awake",
      "CI health",
    ]) {
      expect(heading(name)).toBeInTheDocument();
    }
    expect(section("Summary")).toHaveAttribute("data-tour", "tiles");
    expect(container.querySelector('[data-tour="needs"]')).toHaveAttribute(
      "aria-labelledby",
      "h-needs",
    );
    expect(container.querySelector('[data-tour="activity"]')).toHaveAttribute(
      "aria-labelledby",
      "h-act",
    );
  });

  it("fills its parent and scrolls vertically", () => {
    const { container } = render(() => <HomeView />);
    expect(container.firstElementChild).toHaveClass(
      "absolute",
      "inset-0",
      "overflow-y-auto",
      "overflow-x-hidden",
    );
  });

  it("uses two columns of tiles and 16 px padding on phones", () => {
    M.setViewport(PHONE_WIDTH_PX, 800);
    const { container } = render(() => <HomeView />);
    expect(container.querySelector(".grid-cols-2")).toBeInTheDocument();
    expect(container.querySelector(".px-4.pt-4.pb-8")).toBeInTheDocument();
  });

  it("stacks the section pairs and the charts below 900 and 1000 px", () => {
    M.setViewport(TABLET_WIDTH_PX, 900);
    const { container } = render(() => <HomeView />);
    expect(container.querySelector(".grid-cols-4")).toBeInTheDocument();
    expect(container.querySelectorAll(".grid-cols-1")).toHaveLength(3);
    expect(container.querySelector(".px-6.pt-6.pb-12")).toBeInTheDocument();
  });

  it("puts the sections in two columns from 900 px and the charts side by side from 1000", () => {
    render(() => <HomeView />);
    const pair = heading("Recent activity").closest("section")?.parentElement;
    expect(pair).toHaveClass("grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]");
    expect(screen.getAllByRole("img")[0]?.closest("figure")?.parentElement).toHaveClass(
      "grid-cols-2",
    );
  });
});

describe("summary tiles", { timeout: SLOW_TEST_MS }, () => {
  it("shows what needs you, what works, what merged, and the cost", () => {
    render(() => <HomeView />);
    const tiles = within(section("Summary"));
    const need = tiles.getByRole("button", { name: /Need you$/ });
    expect(need).toHaveTextContent(`${M.needs().length}Need you`);
    expect(need).toHaveAttribute("title", "Open the cards that need you");
    const working = tiles.getByRole("button", { name: /Working now$/ });
    expect(working).toHaveTextContent(`${M.working().length}Working now`);
    expect(working).toHaveAttribute("title", "Open working cards");
    expect(tiles.getByRole("button", { name: /Merged today$/ })).toHaveAttribute(
      "title",
      "Open done cards",
    );
    const cost = M.costs();
    const tile = tiles.getByRole("button", { name: /Cost today of/ });
    expect(tile).toHaveTextContent(`${M.money(cost.today)}Cost today of ${M.money(cost.day)}`);
    expect(tile).toHaveAttribute("title", "Open cost limits");
  });

  it("colors the need count amber, and the cost by how near the limit is", () => {
    render(() => <HomeView />);
    const need = screen.getByRole("button", { name: /Need you$/ });
    expect(within(need).getByText(String(M.needs().length))).toHaveClass(
      "text-status-needs-you-text",
    );
    const cost = () =>
      within(screen.getByRole("button", { name: /Cost today of/ })).getByText(/^\$\d/);
    expect(cost()).toHaveClass("text-primary");
    M.S.limits.global.day = Math.ceil(M.costs().today / 0.9);
    expect(cost()).toHaveClass("text-status-needs-you-text");
    M.S.limits.global.day = Math.floor(M.costs().today / 2);
    expect(cost()).toHaveClass("text-status-danger-text");
  });

  it("colors the need count plainly when nothing needs you", () => {
    setNeedsCardsTo("working");
    render(() => <HomeView />);
    const need = screen.getByRole("button", { name: /Need you$/ });
    expect(within(need).getByText("0")).toHaveClass("text-primary");
  });

  it("opens the list of the busiest project filtered to the tile's status", () => {
    render(() => <HomeView />);
    fireEvent.click(screen.getByRole("button", { name: /Need you$/ }));
    expect(M.S.route).toMatchObject({ page: "project", pid: "api", view: "list" });
    expect(M.S.filters.api).toEqual([{ k: "status", v: "needs" }]);
    expect(M.S.savedView.api).toBeNull();
    M.go("home");
    fireEvent.click(screen.getByRole("button", { name: /Working now$/ }));
    expect(M.S.filters[M.S.route.pid ?? ""]).toEqual([{ k: "status", v: "working" }]);
    M.go("home");
    fireEvent.click(screen.getByRole("button", { name: /Merged today$/ }));
    expect(M.S.filters[M.S.route.pid ?? ""]).toEqual([{ k: "status", v: "done" }]);
  });

  it("opens the cost limits in settings", () => {
    render(() => <HomeView />);
    fireEvent.click(screen.getByRole("button", { name: /Cost today of/ }));
    expect(M.S.route.page).toBe("settings");
    expect(M.S.settingsSection).toBe("limits");
  });
});

describe("charts", { timeout: SLOW_TEST_MS }, () => {
  it("names each chart by its summary and draws seven bars for a week", () => {
    const { container } = render(() => <HomeView />);
    const bars = screen.getByRole("img", { name: /cards finished in the last 7 days$/ });
    expect(bars.querySelectorAll("path")).toHaveLength(7);
    const cost = M.costs();
    expect(
      screen.getByRole("img", {
        name: `All projects today ${M.money(cost.today)} of the ${M.money(cost.day)} daily limit`,
      }),
    ).toBeInTheDocument();
    expect(screen.getByText("Cards finished per day")).toBeInTheDocument();
    expect(screen.getByText("Cost per project per day")).toBeInTheDocument();
    expect(container.querySelectorAll("figure")).toHaveLength(2);
  });

  it("lists the projects in the legend, with the daily limit last", () => {
    render(() => <HomeView />);
    const legend = screen.getByText("Daily limit").parentElement;
    const names = [...(legend?.children ?? [])].map((entry) => entry.textContent);
    expect(names).toEqual([
      "All projects",
      "api-gateway",
      "web-dashboard",
      "mobile-app",
      "Daily limit",
    ]);
  });

  it("draws the limit label next to the limit line", () => {
    render(() => <HomeView />);
    expect(screen.getByText(`Limit ${M.money(M.costs().day)}`)).toBeInTheDocument();
  });

  it("starts at 520 px wide, the width the design assumes before it measures", () => {
    render(() => <HomeView />);
    expect(screen.getAllByRole("img")[0]).toHaveAttribute("width", "520");
  });

  it("switches range with the segmented control", () => {
    render(() => <HomeView />);
    const group = screen.getByRole("radiogroup", { name: "Chart range" });
    const radios = within(group).getAllByRole("radio");
    expect(radios.map((r) => r.textContent)).toEqual(["7 days", "30 days", "90 days"]);
    expect(radios[0]).toHaveAttribute("aria-checked", "true");
    fireEvent.click(radios[2] as HTMLElement);
    expect(M.S.dashRange).toBe(90);
    expect(radios[2]).toHaveAttribute("aria-checked", "true");
    const bars = screen.getByRole("img", { name: /cards finished in the last 90 days$/ });
    expect(bars.querySelectorAll("path")).toHaveLength(90);
  });

  it("makes the range segments 40 px high under 1200 px", () => {
    M.setViewport(1000, 900);
    render(() => <HomeView />);
    for (const radio of screen.getAllByRole("radio")) expect(radio).toHaveClass("h-10");
    expect(screen.getAllByRole("radio")[0]).toHaveAttribute("data-compact", "1");
  });

  it("keeps them 26 px high on wide windows", () => {
    render(() => <HomeView />);
    expect(screen.getAllByRole("radio")[0]).toHaveClass("h-6.5");
  });
});

describe("Needs you", { timeout: SLOW_TEST_MS }, () => {
  it("groups the waiting cards by project with a count", () => {
    render(() => <HomeView />);
    const needs = within(section("Needs you"));
    expect(needs.getByRole("button", { name: "api-gateway2 cards" })).toBeVisible();
    expect(needs.getByRole("button", { name: "web-dashboard1 card" })).toBeVisible();
    expect(needs.getByText(String(M.needs().length))).toHaveClass("bg-status-needs-you-subtle");
  });

  it("gives each card its reason, title, number, owner, and wait", () => {
    render(() => <HomeView />);
    const needs = within(section("Needs you"));
    expect(needs.getByText("Plan ready for review")).toHaveClass("font-semibold");
    expect(needs.getByText("Add rate limiting per API key")).toBeVisible();
    expect(needs.getByText("#43")).toHaveClass("text-muted");
    const wait = needs.getByText("Waiting since 34 min ago");
    expect(wait).toHaveAttribute("title", M.full(M.card(43)?.upd ?? 0));
  });

  it("names the button after what the card asks", () => {
    render(() => <HomeView />);
    const needs = within(section("Needs you"));
    expect(needs.getByRole("button", { name: "Review plan" })).toBeVisible();
    expect(needs.getByRole("button", { name: "Review command" })).toBeVisible();
    expect(needs.getByRole("button", { name: "Reply" })).toBeVisible();
    expect(needs.getByRole("button", { name: "Resolve conflict" })).toBeVisible();
  });

  it("offers Approve only for a waiting command, and approving works the card", () => {
    render(() => <HomeView />);
    const approve = screen.getAllByRole("button", { name: "Approve" });
    expect(approve).toHaveLength(1);
    expect(approve[0]).toHaveClass("bg-ink", "hover:bg-ink!");
    fireEvent.click(approve[0] as HTMLElement);
    expect(M.card(44)?.state).toBe("working");
    expect(screen.queryByRole("button", { name: "Approve" })).not.toBeInTheDocument();
  });

  it("opens the card from its row and its button, and the project from its heading", () => {
    render(() => <HomeView />);
    fireEvent.click(screen.getByRole("button", { name: "Review plan" }));
    expect(M.S.openId).toBe(43);
    M.S.openId = null;
    fireEvent.click(screen.getByText("Add rate limiting per API key"));
    expect(M.S.openId).toBe(43);
    fireEvent.click(screen.getByRole("button", { name: "web-dashboard1 card" }));
    expect(M.S.route).toMatchObject({ page: "project", pid: "web" });
  });

  it("says nothing needs you, and counts the working agents", () => {
    setNeedsCardsTo("working");
    render(() => <HomeView />);
    const working = M.working().length;
    expect(
      screen.getByText(`Nothing needs you right now. ${working} agents are working.`),
    ).toBeVisible();
    expect(within(section("Needs you")).queryByText("0")).not.toBeInTheDocument();
  });

  it("uses the singular for one working agent", () => {
    setNeedsCardsTo("review");
    for (const card of M.S.cards.filter((c) => c.state === "working").slice(1)) {
      card.state = "review";
    }
    render(() => <HomeView />);
    expect(screen.getByText("Nothing needs you right now. 1 agent is working.")).toBeVisible();
  });
});
