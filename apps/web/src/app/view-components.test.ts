import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const stubs = vi.hoisted(() => {
  const stub = () => () => document.createElement("div");
  return {
    AgentsView: stub(),
    BoardView: stub(),
    CalendarView: stub(),
    ChatsView: stub(),
    HomeAllView: stub(),
    HomeView: stub(),
    ListView: stub(),
    SettingsView: stub(),
    TimelineView: stub(),
  };
});
vi.mock("~/views/agents/AgentsView", () => ({ AgentsView: stubs.AgentsView }));
vi.mock("~/views/board/BoardView", () => ({ BoardView: stubs.BoardView }));
vi.mock("~/views/calendar/CalendarView", () => ({ CalendarView: stubs.CalendarView }));
vi.mock("~/views/chats/ChatsView", () => ({ ChatsView: stubs.ChatsView }));
vi.mock("~/views/home/HomeAllView", () => ({ HomeAllView: stubs.HomeAllView }));
vi.mock("~/views/home/HomeView", () => ({ HomeView: stubs.HomeView }));
vi.mock("~/views/list/ListView", () => ({ ListView: stubs.ListView }));
vi.mock("~/views/settings/SettingsView", () => ({ SettingsView: stubs.SettingsView }));
vi.mock("~/views/timeline/TimelineView", () => ({ TimelineView: stubs.TimelineView }));

const { routeComponent, VIEW_COMPONENTS } = await import("./view-components");

describe("VIEW_COMPONENTS", () => {
  it("maps each of the six project views to its component", () => {
    expect(VIEW_COMPONENTS.chat).toBe(stubs.ChatsView);
    expect(VIEW_COMPONENTS.agents).toBe(stubs.AgentsView);
    expect(VIEW_COMPONENTS.board).toBe(stubs.BoardView);
    expect(VIEW_COMPONENTS.list).toBe(stubs.ListView);
    expect(VIEW_COMPONENTS.timeline).toBe(stubs.TimelineView);
    expect(VIEW_COMPONENTS.calendar).toBe(stubs.CalendarView);
  });
});

describe("routeComponent", () => {
  beforeEach(() => M.go("home"));

  it("picks the page component for home, all, and settings", () => {
    expect(routeComponent()).toBe(stubs.HomeView);
    M.go("all");
    expect(routeComponent()).toBe(stubs.HomeAllView);
    M.go("settings");
    expect(routeComponent()).toBe(stubs.SettingsView);
  });

  it("picks the project's current view", () => {
    M.go("project", "api", "list");
    expect(routeComponent()).toBe(stubs.ListView);
    M.setView("calendar");
    expect(routeComponent()).toBe(stubs.CalendarView);
  });

  it("shows nothing for a project that no longer exists", () => {
    M.go("project", "gone", "board");
    expect(routeComponent()).toBeUndefined();
  });
});
