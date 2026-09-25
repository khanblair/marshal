import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { Sidebar } from "./Sidebar";
import { resetShell, TABLET_PX } from "./shell-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const projectLabel = (id: string): string => {
  const project = M.proj(id);
  if (!project) throw new Error(`no project ${id}`);
  const ciText = project.ci ? `, main CI ${M.CI[project.ci].label.toLowerCase()}` : "";
  return `${project.name}: ${M.needs(id).length} need you${ciText}, ${M.awake(id).length} awake agents`;
};

beforeEach(() => resetShell());
afterEach(cleanup);

describe("Sidebar (desktop, expanded)", () => {
  it("is the main navigation with a labeled project list", () => {
    render(() => <Sidebar />);
    const nav = screen.getByRole("navigation", { name: "Main" });
    expect(within(nav).getByText("Marshal")).toBeInTheDocument();
    const heading = screen.getByText("Projects");
    expect(heading).toHaveAttribute("id", "projects-heading");
    const list = screen.getByRole("list", { name: "Projects" });
    expect(list).toHaveAttribute("data-tour", "projects");
    expect(within(list).getAllByRole("listitem")).toHaveLength(M.S.projects.length);
  });

  it("describes each project in its button's name and title", () => {
    render(() => <Sidebar />);
    for (const project of M.S.projects) {
      const button = screen.getByRole("button", { name: projectLabel(project.id) });
      expect(button).toHaveAttribute("title", projectLabel(project.id));
      expect(button).toHaveTextContent(project.name);
    }
  });

  it("shows the needs-you count on Home with a title, and marks Home current", () => {
    render(() => <Sidebar />);
    const home = screen.getByRole("button", { name: /^Home/ });
    expect(home).toHaveAttribute("aria-current", "page");
    expect(
      within(home).getByTitle(`${M.needs().length} cards need you across all projects`),
    ).toBeInTheDocument();
  });

  it("goes to a project and marks it selected", () => {
    render(() => <Sidebar />);
    fireEvent.click(screen.getByRole("button", { name: projectLabel("web") }));
    expect(M.S.route).toMatchObject({ page: "project", pid: "web" });
    expect(screen.getByRole("button", { name: /^Home/ })).not.toHaveAttribute("aria-current");
    const row = screen
      .getByRole("button", { name: projectLabel("web") })
      .closest("[role=listitem]");
    expect(row).toHaveClass("bg-surface-selected");
    expect(row?.querySelector(".bg-ink")).not.toBeNull();
  });

  it("opens New project with an empty draft", () => {
    render(() => <Sidebar />);
    const button = screen.getByRole("button", { name: "New project" });
    expect(button).toHaveAttribute("data-tour", "new-project");
    fireEvent.click(button);
    expect(M.S.newProject).toMatchObject({ source: "folder", branch: "" });
  });

  it("goes to Settings and marks it current", () => {
    render(() => <Sidebar />);
    fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    expect(M.S.route.page).toBe("settings");
    expect(screen.getByRole("button", { name: "Settings" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("collapses with the header button", () => {
    render(() => <Sidebar />);
    const collapse = screen.getByRole("button", { name: "Collapse sidebar" });
    expect(collapse).toHaveAttribute("title", "Collapse sidebar");
    fireEvent.click(collapse);
    expect(M.S.sidebarCollapsed).toBe(true);
  });
});

describe("Sidebar (desktop, collapsed)", () => {
  beforeEach(() => M.set({ sidebarCollapsed: true }));

  it("shows icons only, with the names still on the buttons", () => {
    render(() => <Sidebar />);
    expect(screen.queryByText("Marshal")).toBeNull();
    expect(screen.queryByText("Projects")).toBeNull();
    expect(screen.queryByText("New project")).toBeNull();
    expect(screen.queryByText("Settings")).toBeNull();
    expect(screen.getByRole("button", { name: "New project" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Settings" })).toHaveAttribute("title", "Settings");
    expect(screen.queryByRole("button", { name: /More actions for/ })).toBeNull();
  });

  it("shows the needs-you count over each initial", () => {
    render(() => <Sidebar />);
    const api = screen.getByRole("button", { name: projectLabel("api") });
    expect(api).toHaveTextContent(`A${M.needs("api").length}`);
    expect(api).not.toHaveTextContent("api-gateway");
  });

  it("expands with its own button", () => {
    render(() => <Sidebar />);
    fireEvent.click(screen.getByRole("button", { name: "Expand sidebar" }));
    expect(M.S.sidebarCollapsed).toBe(false);
  });
});

describe("Sidebar (tablet)", () => {
  beforeEach(() => M.setViewport(TABLET_PX, 900));

  it("is a narrow icon bar until the overlay opens", () => {
    render(() => <Sidebar />);
    expect(screen.queryByText("Projects")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Expand sidebar" }));
    expect(M.S.sideOpen).toBe(true);
    expect(screen.getByText("Projects")).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "Main" })).toHaveClass("z-side", "shadow-e2");
  });

  it("closes the overlay from its scrim and from its collapse button", () => {
    M.set({ sideOpen: true });
    const { container } = render(() => <Sidebar />);
    const scrim = container.querySelector<HTMLElement>(".bg-scrim-side");
    expect(scrim).toHaveClass("w-screen", "z-[155]");
    if (scrim) fireEvent.click(scrim);
    expect(M.S.sideOpen).toBe(false);
    M.set({ sideOpen: true });
    fireEvent.click(screen.getByRole("button", { name: "Collapse sidebar" }));
    expect(M.S.sideOpen).toBe(false);
  });

  it("closes the overlay when a project is chosen", () => {
    M.set({ sideOpen: true });
    render(() => <Sidebar />);
    fireEvent.click(screen.getByRole("button", { name: projectLabel("mobile") }));
    expect(M.S.sideOpen).toBe(false);
    expect(M.S.route.pid).toBe("mobile");
  });
});
