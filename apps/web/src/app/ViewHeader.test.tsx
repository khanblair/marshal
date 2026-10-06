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
  it("lists the seven views with their shortcuts and marks the current one", () => {
    render(() => <ViewHeader />);
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual([
      "Chats",
      "Agents",
      "Board",
      "List",
      "Timeline",
      "Calendar",
      "Integration",
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

  it("sizes the tabs the same on every layout, and lets the base CSS raise them on touch tablets", () => {
    showProject("board", TABLET_PX);
    render(() => <ViewHeader />);
    expect(screen.getByRole("tab", { name: "Board" })).toHaveClass("h-6.5");
    expect(screen.getByRole("tab", { name: "Board" })).not.toHaveAttribute("data-compact");
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

describe("ViewHeader More menu", () => {
  const more = () => screen.getByRole("button", { name: "More project actions" });

  it.each(["board", "list", "timeline"] as const)(
    "offers the board's two Google exports in the %s view, after New card",
    (view) => {
      showProject(view);
      render(() => <ViewHeader />);
      expect(more()).toHaveAttribute("aria-expanded", "false");
      fireEvent.click(more());
      expect(more()).toHaveAttribute("aria-expanded", "true");
      expect(screen.getAllByRole("menuitem").map((item) => item.textContent)).toEqual([
        "Export board to Google Sheet",
        "Export board to Google Slides",
      ]);
      const buttons = screen
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label") ?? button.textContent);
      expect(buttons.indexOf("More project actions")).toBeGreaterThan(
        buttons.findIndex((name) => /New card/.test(name ?? "")),
      );
    },
  );

  it("is drawn where the other views are, but not where the header is not", () => {
    showProject("chat");
    render(() => <ViewHeader />);
    expect(more()).toBeInTheDocument();
    cleanup();
    showProject("board", PHONE_PX);
    render(() => <ViewHeader />);
    expect(screen.queryByRole("button", { name: "More project actions" })).toBeNull();
  });

  it("closes, and says to connect the service, when an export is chosen without it", () => {
    render(() => <ViewHeader />);
    fireEvent.click(more());
    fireEvent.click(screen.getByRole("menuitem", { name: "Export board to Google Sheet" }));
    expect(M.S.menu).toBeNull();
    expect(screen.queryByRole("menuitem")).toBeNull();
    expect(M.S.toasts.at(-1)?.msg).toBe("Connect Google Sheets in Settings, under Integrations.");
    fireEvent.click(more());
    fireEvent.click(screen.getByRole("menuitem", { name: "Export board to Google Slides" }));
    expect(M.S.toasts.at(-1)?.msg).toBe("Connect Google Slides in Settings, under Integrations.");
  });

  it("is the same size on every layout, and lets the base CSS raise it on touch tablets", () => {
    showProject("board", TABLET_PX);
    render(() => <ViewHeader />);
    expect(more()).toHaveClass("size-7");
    expect(more()).not.toHaveAttribute("data-compact");
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

describe("ViewHeader merge target", () => {
  const chip = () => screen.queryByTitle("Finished cards merge into development");
  afterEach(() => {
    const project = M.proj("api");
    if (project) delete project.integrationBranch;
  });

  it("says which branch finished cards merge into", () => {
    Object.assign(M.proj("api") ?? {}, { integrationBranch: "development" });
    render(() => <ViewHeader />);
    expect(chip()).toHaveTextContent("Merging into development");
  });

  it("follows a change of the branch", () => {
    Object.assign(M.proj("api") ?? {}, { integrationBranch: "development" });
    render(() => <ViewHeader />);
    const project = M.proj("api");
    if (project) project.integrationBranch = "trunk";
    expect(chip()).toBeNull();
    expect(screen.getByTitle("Finished cards merge into trunk")).toHaveTextContent(
      "Merging into trunk",
    );
  });

  it("is not drawn for a project with no branch known, or outside a project", () => {
    render(() => <ViewHeader />);
    expect(screen.queryByText(/Merging into/)).toBeNull();
    cleanup();
    Object.assign(M.proj("api") ?? {}, { integrationBranch: "development" });
    M.go("home");
    render(() => <ViewHeader />);
    expect(screen.queryByText(/Merging into/)).toBeNull();
  });

  it("truncates on a narrow header", () => {
    Object.assign(M.proj("api") ?? {}, { integrationBranch: "development" });
    showProject("board", TABLET_PX);
    render(() => <ViewHeader />);
    expect(chip()).toHaveClass("min-w-0", "max-w-60", "@max-[720px]:max-w-32");
  });
});
