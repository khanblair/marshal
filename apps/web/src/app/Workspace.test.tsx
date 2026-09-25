import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M, type ViewKey } from "~/mock";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/** Plain DOM stand-ins for the views, so these tests do not depend on their code. */
const stubs = vi.hoisted(() => {
  const stub = (name: string) => () => {
    const el = document.createElement("div");
    el.dataset.view = name;
    return el;
  };
  return {
    agents: stub("agents"),
    board: stub("board"),
    calendar: stub("calendar"),
    card: stub("card"),
    chat: stub("chat"),
    homeAll: stub("home-all"),
    home: stub("home"),
    list: stub("list"),
    settings: stub("settings"),
    timeline: stub("timeline"),
  };
});
vi.mock("~/views/agents/AgentsView", () => ({ AgentsView: stubs.agents }));
vi.mock("~/views/board/BoardView", () => ({ BoardView: stubs.board }));
vi.mock("~/views/calendar/CalendarView", () => ({ CalendarView: stubs.calendar }));
vi.mock("~/views/card/CardDetail", () => ({ CardDetail: stubs.card }));
vi.mock("~/views/chats/ChatsView", () => ({ ChatsView: stubs.chat }));
vi.mock("~/views/home/HomeAllView", () => ({ HomeAllView: stubs.homeAll }));
vi.mock("~/views/home/HomeView", () => ({ HomeView: stubs.home }));
vi.mock("~/views/list/ListView", () => ({ ListView: stubs.list }));
vi.mock("~/views/settings/SettingsView", () => ({ SettingsView: stubs.settings }));
vi.mock("~/views/timeline/TimelineView", () => ({ TimelineView: stubs.timeline }));

const { Workspace } = await import("./Workspace");

const DESKTOP_PX = 1440;
const TABLET_PX = 820;
const PHONE_PX = 390;

function showProject(view: ViewKey, width = DESKTOP_PX): void {
  M.setViewport(width, 900);
  M.set({ split: [], openId: null, detailExpanded: false, menu: null });
  M.go("project", "api", view);
}

const shown = (): string[] =>
  Array.from(document.querySelectorAll<HTMLElement>("[data-view]")).map(
    (el) => el.dataset.view ?? "",
  );
const main = (): HTMLElement => screen.getByRole("main");

beforeEach(() => showProject("board"));
afterEach(cleanup);

describe("Workspace main view", () => {
  it.each([
    ["chat", "chat"],
    ["agents", "agents"],
    ["board", "board"],
    ["list", "list"],
    ["timeline", "timeline"],
    ["calendar", "calendar"],
  ] as const)("draws the %s view for the project's %s tab", (view, name) => {
    showProject(view);
    render(() => <Workspace />);
    expect(within(main()).getByText("", { selector: `[data-view="${name}"]` })).toBeTruthy();
    expect(shown()).toEqual([name]);
  });

  it("draws Home, the All page, and Settings", () => {
    M.go("home");
    render(() => <Workspace />);
    expect(shown()).toEqual(["home"]);
    M.go("all");
    expect(shown()).toEqual(["home-all"]);
    M.go("settings");
    expect(shown()).toEqual(["settings"]);
  });

  it("follows the route as it changes", () => {
    render(() => <Workspace />);
    M.setView("list");
    expect(shown()).toEqual(["list"]);
  });

  it("keeps an empty main for a project that no longer exists", () => {
    M.go("project", "gone", "board");
    render(() => <Workspace />);
    expect(main()).toBeEmptyDOMElement();
  });

  it("fills a flex row that clips its children", () => {
    const { container } = render(() => <Workspace />);
    expect(container.firstElementChild).toHaveClass("flex-1", "min-h-0", "flex", "relative");
    expect(main()).toHaveClass("flex-1", "min-w-0", "overflow-hidden");
  });
});

describe("Workspace split panes", () => {
  it("draws a titled pane per split view beside the main view on desktop", () => {
    M.set({ split: ["list", "calendar"] });
    render(() => <Workspace />);
    expect(shown()).toEqual(["board", "list", "calendar"]);
    const first = screen.getByRole("region", { name: "Pane 2" });
    expect(within(first).getByRole("combobox", { name: "View in this pane" })).toHaveValue("list");
    expect(screen.getByRole("region", { name: "Pane 3" })).toBeInTheDocument();
  });

  it("offers every view in each pane's picker and switches the pane", () => {
    M.set({ split: ["list"] });
    render(() => <Workspace />);
    const picker = screen.getByRole("combobox", { name: "View in this pane" });
    expect(
      within(picker)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["Chats", "Agents", "Board", "List", "Timeline", "Calendar"]);
    fireEvent.change(picker, { target: { value: "timeline" } });
    expect([...M.S.split]).toEqual(["timeline"]);
    expect(shown()).toEqual(["board", "timeline"]);
  });

  it("ignores a picker value that is not a view", () => {
    M.set({ split: ["list"] });
    render(() => <Workspace />);
    fireEvent.change(screen.getByRole("combobox", { name: "View in this pane" }), {
      target: { value: "nope" },
    });
    expect([...M.S.split]).toEqual(["list"]);
  });

  it("closes a pane", () => {
    M.set({ split: ["list", "calendar"] });
    render(() => <Workspace />);
    const first = screen.getByRole("region", { name: "Pane 2" });
    fireEvent.click(within(first).getByRole("button", { name: "Close pane" }));
    expect([...M.S.split]).toEqual(["calendar"]);
    expect(shown()).toEqual(["board", "calendar"]);
  });

  it("draws only the first three panes on desktop and one on tablet", () => {
    M.set({ split: ["chat", "list", "timeline", "calendar"] });
    render(() => <Workspace />);
    expect(screen.getAllByRole("region")).toHaveLength(3);
    cleanup();
    showProject("board", TABLET_PX);
    M.set({ split: ["chat", "list"] });
    render(() => <Workspace />);
    expect(screen.getAllByRole("region")).toHaveLength(1);
  });

  it("draws none on phones, outside projects, or while the card is expanded", () => {
    showProject("board", PHONE_PX);
    M.set({ split: ["list"] });
    render(() => <Workspace />);
    expect(screen.queryByRole("region")).toBeNull();
    cleanup();
    showProject("board");
    M.set({ split: ["list"] });
    M.go("home");
    render(() => <Workspace />);
    expect(screen.queryByRole("region")).toBeNull();
  });
});

describe("Workspace card panel", () => {
  it("shows the card beside the main view on desktop", () => {
    M.openCard("api#41");
    render(() => <Workspace />);
    expect(shown()).toEqual(["board", "card"]);
    expect(screen.getByRole("complementary", { name: "Card detail" })).toBeInTheDocument();
    expect(screen.getByRole("separator", { name: "Resize card panel" })).toBeInTheDocument();
  });

  it("hides the main view and the panes while the card is expanded", () => {
    M.set({ split: ["list"] });
    M.openCard("api#41");
    M.set({ detailExpanded: true });
    render(() => <Workspace />);
    expect(screen.queryByRole("main")).toBeNull();
    expect(screen.queryByRole("region")).toBeNull();
    expect(shown()).toEqual(["card"]);
    expect(screen.queryByRole("separator")).toBeNull();
  });

  it("covers the main view on phones", () => {
    showProject("board", PHONE_PX);
    M.openCard("api#41");
    render(() => <Workspace />);
    expect(screen.queryByRole("main")).toBeNull();
    expect(shown()).toEqual(["card"]);
  });

  it("overlays the main view on tablets, and the backdrop closes the card", () => {
    showProject("board", TABLET_PX);
    M.openCard("api#41");
    const { container } = render(() => <Workspace />);
    expect(shown()).toEqual(["board", "card"]);
    const backdrop = container.querySelector<HTMLElement>('[aria-hidden="true"].z-\\[140\\]');
    expect(backdrop).not.toBeNull();
    fireEvent.click(backdrop as HTMLElement);
    expect(M.S.openId).toBeNull();
    expect(shown()).toEqual(["board"]);
  });

  it("shows nothing for an id that is not a card", () => {
    M.set({ openId: "api#99999" });
    render(() => <Workspace />);
    expect(screen.queryByRole("complementary")).toBeNull();
    expect(shown()).toEqual(["board"]);
  });
});
