import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { ProjectRow } from "./ProjectRow";
import { resetShell } from "./shell-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

function api() {
  const project = M.proj("api");
  if (!project) throw new Error("seed project missing");
  return project;
}

const renderRow = () =>
  render(() => (
    <ul>
      <ProjectRow project={api()} />
    </ul>
  ));

beforeEach(() => {
  resetShell();
  vi.useFakeTimers();
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("ProjectRow menu", () => {
  it("opens and closes its actions menu", () => {
    renderRow();
    const more = screen.getByRole("button", { name: "More actions for api-gateway" });
    expect(more).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(more);
    expect(M.S.menu).toBe("proj:api");
    expect(more).toHaveAttribute("aria-expanded", "true");
    expect(screen.getAllByRole("menuitem").map((i) => i.textContent)).toEqual([
      "Rename",
      "Project settings",
      "Remove",
    ]);
    fireEvent.click(more);
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("Project settings opens the project's Settings section", () => {
    M.set({ menu: "proj:api" });
    renderRow();
    fireEvent.click(screen.getByRole("menuitem", { name: "Project settings" }));
    expect(M.S).toMatchObject({ settingsSection: "project", settingsPid: "api", menu: null });
    expect(M.S.route.page).toBe("settings");
  });

  it("Remove asks to confirm", () => {
    M.set({ menu: "proj:api" });
    renderRow();
    fireEvent.click(screen.getByRole("menuitem", { name: "Remove" }));
    expect(M.S.removeProject).toEqual({ id: "api", keepBranches: true, keepMemory: true });
  });
});

describe("ProjectRow rename", () => {
  // The daemon call is tested with a daemon in memory (`sync/project-actions.test.ts`). Here the
  // action stands in for its optimistic step: the name changes on the screen at once.
  beforeEach(() => {
    vi.spyOn(M, "renameProject").mockImplementation(async (id, name) => {
      const project = M.proj(id);
      if (project) project.name = name.trim();
      return true;
    });
  });

  const startRename = () => {
    M.set({ menu: "proj:api" });
    renderRow();
    fireEvent.click(screen.getByRole("menuitem", { name: "Rename" }));
    vi.advanceTimersByTime(1);
    return screen.getByRole("textbox", { name: "Project name" }) as HTMLInputElement;
  };

  it("swaps the row for a focused field with the name selected", () => {
    const field = startRename();
    expect(M.S.renaming).toBe("api");
    expect(field).toHaveValue("api-gateway");
    expect(document.activeElement).toBe(field);
    expect(field.selectionStart).toBe(0);
    expect(field.selectionEnd).toBe("api-gateway".length);
    expect(screen.queryByRole("button", { name: /More actions/ })).toBeNull();
  });

  it("saves on Enter", () => {
    const field = startRename();
    fireEvent.input(field, { target: { value: "billing" } });
    fireEvent.keyDown(field, { key: "Enter" });
    expect(M.S.renaming).toBeNull();
    expect(M.renameProject).toHaveBeenCalledWith("api", "billing");
    expect(api().name).toBe("billing");
    expect(screen.getByRole("button", { name: /^billing: / })).toBeInTheDocument();
  });

  it("saves when the field loses focus", () => {
    const field = startRename();
    fireEvent.input(field, { target: { value: "gateway-v2" } });
    fireEvent.blur(field);
    expect(api().name).toBe("gateway-v2");
  });

  it("cancels on Escape and keeps the key from reaching the app", () => {
    const field = startRename();
    const onWindowKey = vi.fn();
    window.addEventListener("keydown", onWindowKey);
    fireEvent.input(field, { target: { value: "scratch" } });
    fireEvent.keyDown(field, { key: "Escape" });
    window.removeEventListener("keydown", onWindowKey);
    expect(onWindowKey).not.toHaveBeenCalled();
    expect(M.S.renaming).toBeNull();
    expect(api().name).toBe("api-gateway");
  });

  it("keeps the old name and says so when the new one is empty", () => {
    const field = startRename();
    vi.mocked(M.renameProject).mockRestore();
    fireEvent.input(field, { target: { value: "   " } });
    fireEvent.keyDown(field, { key: "Enter" });
    expect(api().name).toBe("api-gateway");
    expect(M.S.toasts.at(-1)?.msg).toBe("Project names can't be empty. The old name is kept.");
  });

  it("does nothing when the name is unchanged", () => {
    const field = startRename();
    fireEvent.keyDown(field, { key: "Enter" });
    expect(M.S.renaming).toBeNull();
    expect(M.S.toasts).toHaveLength(0);
  });

  it("ends once: blur after Enter changes nothing more", () => {
    const field = startRename();
    fireEvent.input(field, { target: { value: "one" } });
    fireEvent.keyDown(field, { key: "Enter" });
    fireEvent.input(field, { target: { value: "two" } });
    fireEvent.blur(field);
    expect(M.renameProject).toHaveBeenCalledOnce();
    expect(api().name).toBe("one");
  });
});

describe("ProjectRow content", () => {
  it("shows the initial, the needs-you count, the CI state, and the awake count", () => {
    renderRow();
    const button = screen.getByRole("button", { name: /^api-gateway: / });
    expect(button).toHaveTextContent("A");
    expect(button).toHaveTextContent(String(M.needs("api").length));
    expect(button).toHaveTextContent(String(M.awake("api").length));
  });

  it("draws no CI state, and says nothing of one, for a project the daemon has no CI data for", () => {
    const { unmount } = render(() => (
      <ul>
        <ProjectRow
          project={{ id: "billing", name: "billing", lang: "Go", path: "~/code/billing" }}
        />
      </ul>
    ));
    const button = screen.getByRole("button", { name: "billing: 0 need you, 0 awake agents" });
    expect(button).toHaveAttribute("title", "billing: 0 need you, 0 awake agents");
    expect(button.textContent).not.toMatch(/queued|CI/i);
    unmount();
    renderRow();
    expect(
      screen.getByRole("button", { name: /^api-gateway: .*main CI passed/ }),
    ).toBeInTheDocument();
  });

  it("uses a question mark for a nameless project", () => {
    api().name = "";
    renderRow();
    expect(screen.getByRole("button", { name: /^: / })).toHaveTextContent("?");
  });
});
