import { cleanup, fireEvent, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { lastToast, showSettings } from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllTimers();
  vi.useRealTimers();
});

const nameField = () => screen.getByLabelText<HTMLInputElement>("Name");
const saveButton = () => screen.getByRole("button", { name: "Save project" });
const projectPicker = () => screen.getByRole("combobox", { name: "Project" });

describe("Project settings section", () => {
  it("shows the project of the open route, with the design's defaults", () => {
    showSettings("project");
    expect(screen.getByRole("heading", { name: "Project settings", level: 2 })).toBeInTheDocument();
    expect(projectPicker()).toHaveValue("api");
    expect(nameField()).toHaveValue("api-gateway");
    expect(screen.getByLabelText("Default branch")).toHaveValue("main");
    expect(screen.getByLabelText("Default branch")).toHaveClass("font-mono");
    expect(screen.getByLabelText("Dev command", { exact: false })).toHaveValue("");
    expect(screen.getByLabelText("Dev command", { exact: false })).toHaveAttribute(
      "placeholder",
      "pnpm dev",
    );
    expect(screen.getByText("~/code/api-gateway")).toBeInTheDocument();
    expect(screen.getByText("Go project")).toBeInTheDocument();
    expect(screen.getByLabelText("Lock bypass permissions", { exact: false })).not.toBeChecked();
    expect(saveButton()).toBeDisabled();
  });

  it("offers every project, and switches to the one picked", () => {
    showSettings("project");
    const options = Array.from(projectPicker().querySelectorAll("option")).map(
      (o) => o.textContent,
    );
    expect(options).toEqual(["api-gateway", "web-dashboard", "mobile-app"]);
    fireEvent.change(projectPicker(), { target: { value: "web" } });
    expect(M.S.settingsPid).toBe("web");
    expect(nameField()).toHaveValue("web-dashboard");
    expect(screen.getByLabelText("Dev command", { exact: false })).toHaveValue("pnpm dev");
    expect(screen.getByText("TypeScript project")).toBeInTheDocument();
  });

  it("describes a monorepo by its packages", () => {
    showSettings("project", { settingsPid: "mobile" });
    const packages = M.proj("mobile")?.packages?.length ?? 0;
    expect(screen.getByText(`Monorepo with ${packages} packages`)).toBeInTheDocument();
  });

  it("falls back to the first project when the open route has none", () => {
    showSettings("project", { route: { page: "settings", pid: null, view: "board" } });
    expect(projectPicker()).toHaveValue("api");
  });

  // The daemon call itself is tested with a daemon in memory (`sync/project-actions.test.ts`). Here the
  // action stands in for it: it applies the fields at once, as its optimistic step does, and toasts.
  const stubSave = (accept: boolean) =>
    vi.spyOn(M, "saveProject").mockImplementation(async (id, next) => {
      if (!accept) return false;
      const project = M.proj(id);
      if (project) Object.assign(project, { ...next, dev: next.dev });
      M.toast("Project saved");
      return true;
    });

  const editEverything = () => {
    fireEvent.input(nameField(), { target: { value: " gateway " } });
    fireEvent.input(screen.getByLabelText("Default branch"), { target: { value: "develop" } });
    fireEvent.input(screen.getByLabelText("Dev command", { exact: false }), {
      target: { value: "go run ." },
    });
    fireEvent.click(screen.getByLabelText("Lock bypass permissions", { exact: false }));
  };

  it("saves edits to the project and toasts", async () => {
    const save = stubSave(true);
    showSettings("project");
    editEverything();
    expect(saveButton()).toBeEnabled();
    fireEvent.click(saveButton());
    expect(save).toHaveBeenCalledWith("api", {
      name: "gateway",
      branch: "develop",
      dev: "go run .",
      lockBypass: true,
    });
    await vi.waitFor(() => expect(saveButton()).toBeDisabled());
    expect(lastToast()).toBe("Project saved");
    expect(projectPicker().querySelector("option")?.textContent).toBe("gateway");
  });

  it("keeps the person's edits when the daemon does not accept them", async () => {
    const save = stubSave(false);
    showSettings("project");
    editEverything();
    fireEvent.click(saveButton());
    await vi.waitFor(() => expect(save).toHaveBeenCalledOnce());
    expect(nameField()).toHaveValue(" gateway ");
    expect(saveButton()).toBeEnabled();
    expect(M.proj("api")?.name).toBe("api-gateway");
  });

  it("keeps the old name and the edits when the new name is empty", () => {
    showSettings("project");
    fireEvent.input(nameField(), { target: { value: " " } });
    fireEvent.click(saveButton());
    expect(lastToast()).toBe("Project names can't be empty. The old name is kept.");
    expect(M.proj("api")?.name).toBe("api-gateway");
    expect(nameField()).toHaveValue(" ");
  });

  it("drops an edit when another project is picked", () => {
    showSettings("project");
    fireEvent.input(nameField(), { target: { value: "gateway" } });
    fireEvent.change(projectPicker(), { target: { value: "web" } });
    fireEvent.change(projectPicker(), { target: { value: "api" } });
    expect(nameField()).toHaveValue("api-gateway");
  });

  it("opens Remove project for the shown project, keeping branches and memory", () => {
    showSettings("project", { settingsPid: "web" });
    fireEvent.click(screen.getByRole("button", { name: "Remove project" }));
    expect(M.S.removeProject).toEqual({ id: "web", keepBranches: true, keepMemory: true });
  });
});
