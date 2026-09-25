import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M, type ViewKey } from "~/mock";
import { ViewHeader } from "./ViewHeader";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const DESKTOP_PX = 1440;
const TABLET_PX = 820;
const PHONE_PX = 390;

function showProject(view: ViewKey, width = DESKTOP_PX): void {
  M.setViewport(width, 900);
  M.set({ split: [], openId: null, detailExpanded: false, menu: null, newCard: null });
  M.go("project", "api", view);
}

beforeEach(() => showProject("board"));
afterEach(cleanup);

describe("ViewHeader tabs", () => {
  it("lists the six views with their shortcuts and marks the current one", () => {
    render(() => <ViewHeader />);
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual([
      "Chats",
      "Agents",
      "Board",
      "List",
      "Timeline",
      "Calendar",
    ]);
    expect(screen.getByRole("tablist", { name: "Views" })).toHaveAttribute("data-tour", "views");
    const board = screen.getByRole("tab", { name: "Board" });
    expect(board).toHaveAttribute("aria-selected", "true");
    expect(board).toHaveAttribute("title", "Board (Ctrl 3)");
    expect(screen.getByRole("tab", { name: "List" })).toHaveAttribute("aria-selected", "false");
  });

  it("opens the clicked view", () => {
    render(() => <ViewHeader />);
    fireEvent.click(screen.getByRole("tab", { name: "List" }));
    expect(M.S.route.view).toBe("list");
    expect(screen.getByRole("tab", { name: "List" })).toHaveAttribute("aria-selected", "true");
  });

  it("uses taller tabs on touch screens", () => {
    showProject("board", TABLET_PX);
    render(() => <ViewHeader />);
    expect(screen.getByRole("tab", { name: "Board" })).toHaveClass("h-10");
    cleanup();
    showProject("board", DESKTOP_PX);
    render(() => <ViewHeader />);
    expect(screen.getByRole("tab", { name: "Board" })).toHaveClass("h-6.5");
  });

  it("renders nothing on phones or outside a project", () => {
    showProject("board", PHONE_PX);
    render(() => <ViewHeader />);
    expect(screen.queryByRole("tablist")).toBeNull();
    cleanup();
    showProject("board");
    M.go("home");
    render(() => <ViewHeader />);
    expect(screen.queryByRole("tablist")).toBeNull();
  });
});

describe("ViewHeader New card", () => {
  it.each(["board", "list", "timeline", "agents"] as const)("is offered in %s", (view) => {
    showProject(view);
    render(() => <ViewHeader />);
    expect(screen.getByRole("button", { name: /New card/ })).toBeInTheDocument();
  });

  it.each(["chat", "calendar"] as const)("is not offered in %s", (view) => {
    showProject(view);
    render(() => <ViewHeader />);
    expect(screen.queryByRole("button", { name: /New card/ })).toBeNull();
  });

  it("starts a card draft and shows the N key hint on desktop only", () => {
    render(() => <ViewHeader />);
    const button = screen.getByRole("button", { name: /New card/ });
    expect(button.querySelector("kbd")?.textContent).toBe("N");
    fireEvent.click(button);
    expect(M.S.newCard).not.toBeNull();
    cleanup();
    showProject("board", TABLET_PX);
    render(() => <ViewHeader />);
    expect(screen.getByRole("button", { name: /New card/ }).querySelector("kbd")).toBeNull();
  });
});

describe("ViewHeader Split view", () => {
  it("opens the first view that is not on screen, up to three panes on desktop", () => {
    render(() => <ViewHeader />);
    const click = () => fireEvent.click(screen.getByRole("button", { name: "Split view" }));
    click();
    expect([...M.S.split]).toEqual(["chat"]);
    click();
    click();
    expect([...M.S.split]).toEqual(["chat", "agents", "list"]);
    expect(screen.queryByRole("button", { name: "Split view" })).toBeNull();
  });

  it("allows one pane on tablets", () => {
    showProject("board", TABLET_PX);
    render(() => <ViewHeader />);
    fireEvent.click(screen.getByRole("button", { name: "Split view" }));
    expect(M.S.split).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Split view" })).toBeNull();
  });

  it("is hidden while the card panel is expanded", () => {
    M.set({ detailExpanded: true });
    render(() => <ViewHeader />);
    expect(screen.queryByRole("button", { name: "Split view" })).toBeNull();
  });

  it("describes what it does in its title", () => {
    render(() => <ViewHeader />);
    expect(screen.getByRole("button", { name: "Split view" })).toHaveAttribute(
      "title",
      "Open another view beside this one",
    );
  });
});

describe("ViewHeader on a narrow header", () => {
  it("keeps Split view and New card named for screen readers but hides their labels", () => {
    showProject("board", TABLET_PX);
    render(() => <ViewHeader />);
    const row = screen.getAllByRole("tab")[0]?.closest("[data-tour=views]")?.parentElement;
    expect(row).toHaveClass("@container");
    for (const name of ["Split view", "New card"]) {
      const button = screen.getByRole("button", { name });
      expect(button).toHaveClass("@max-[720px]:px-3!");
      expect(screen.getByText(name)).toHaveClass("@max-[720px]:sr-only");
    }
    expect(screen.getByRole("button", { name: "New card" })).toHaveAttribute("title", "New card");
  });
});
