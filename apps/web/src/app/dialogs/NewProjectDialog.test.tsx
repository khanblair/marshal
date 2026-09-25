import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { NewProjectDialog } from "./NewProjectDialog";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const DESKTOP_PX = 1440;
const PHONE_PX = 390;
const HEIGHT_PX = 900;
const EMPTY = { source: "folder", path: "", url: "", name: "", branch: "main" } as const;

beforeEach(() => {
  M.setViewport(DESKTOP_PX, HEIGHT_PX);
  M.set({ newProject: { ...EMPTY }, toasts: [] });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  M.set({ newProject: null, toasts: [] });
  M.go("home");
});

const path = () => screen.getByRole("textbox", { name: /Repository folder/ });
const name = () => screen.getByRole("textbox", { name: "Name" });
const status = () => screen.getByRole("status");
const add = () => screen.getByRole("button", { name: "Add project" });

describe("NewProjectDialog", () => {
  it("renders nothing while there is no draft", () => {
    M.set({ newProject: null });
    render(() => <NewProjectDialog />);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("starts on Pick a folder with empty fields and an idle detection line", () => {
    render(() => <NewProjectDialog />);
    const dialog = screen.getByRole("dialog", { name: "New project" });
    expect(dialog.tagName).toBe("FORM");
    expect(
      screen.getByRole("radiogroup", { name: "Where the repository comes from" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Pick a folder" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "Clone from GitHub" })).not.toBeChecked();
    expect(path()).toHaveAttribute("placeholder", "~/code/my-repo");
    expect(
      screen.getByText(
        "Marshal reads this repository and makes worktrees beside it. It never moves or deletes your files.",
      ),
    ).toBeInTheDocument();
    expect(name()).toHaveValue("");
    expect(screen.getByRole("textbox", { name: "Default branch" })).toHaveValue("main");
    expect(status()).toHaveTextContent(
      "Choose a folder or paste a URL. Marshal detects the language and monorepo tools.",
    );
    expect(add()).toBeDisabled();
  });

  it("fills the name from the folder and reports what it detected", () => {
    render(() => <NewProjectDialog />);
    fireEvent.input(path(), { target: { value: "~/code/billing-service" } });
    expect(name()).toHaveValue("billing-service");
    expect(status()).toHaveTextContent("Detected a Go project on branch main.");
    expect(add()).toBeEnabled();
    fireEvent.input(path(), { target: { value: "~/code/my-monorepo" } });
    expect(status()).toHaveTextContent("Detected a monorepo: pnpm workspaces with 3 packages.");
    fireEvent.input(path(), { target: { value: "~/code/website" } });
    expect(status()).toHaveTextContent("Detected a TypeScript project on branch main.");
  });

  it("stops following the folder once the name is typed", () => {
    render(() => <NewProjectDialog />);
    fireEvent.input(name(), { target: { value: "Payments" } });
    fireEvent.input(path(), { target: { value: "~/code/other" } });
    expect(name()).toHaveValue("Payments");
    expect(M.S.newProject?.nameTouched).toBe(true);
  });

  it("uses the branch in the detection line", () => {
    render(() => <NewProjectDialog />);
    fireEvent.input(path(), { target: { value: "~/code/website" } });
    fireEvent.input(screen.getByRole("textbox", { name: "Default branch" }), {
      target: { value: "develop" },
    });
    expect(status()).toHaveTextContent("Detected a TypeScript project on branch develop.");
  });

  it("fills a sample folder from Choose folder", () => {
    render(() => <NewProjectDialog />);
    fireEvent.click(screen.getByRole("button", { name: "Choose folder" }));
    expect(path()).toHaveValue("~/code/billing-service");
    expect(name()).toHaveValue("billing-service");
  });

  it("switches to a repository URL for GitHub and names the project from it", () => {
    render(() => <NewProjectDialog />);
    fireEvent.click(screen.getByRole("radio", { name: "Clone from GitHub" }));
    expect(screen.queryByRole("textbox", { name: /Repository folder/ })).toBeNull();
    const url = screen.getByRole("textbox", { name: /Repository URL/ });
    expect(url).toHaveAttribute("placeholder", "https://github.com/owner/repo");
    expect(
      screen.getByText("Marshal clones it into ~/code using the GitHub App."),
    ).toBeInTheDocument();
    fireEvent.input(url, { target: { value: "https://github.com/acme/ledger.git" } });
    expect(name()).toHaveValue("ledger");
    expect(add()).toBeEnabled();
  });

  it("switches source with the arrow keys", () => {
    render(() => <NewProjectDialog />);
    fireEvent.keyDown(screen.getByRole("radio", { name: "Pick a folder" }), { key: "ArrowRight" });
    expect(M.S.newProject?.source).toBe("github");
  });

  it("adds the project, opens its board, and closes", () => {
    vi.spyOn(M, "addProject").mockReturnValue("p99");
    render(() => <NewProjectDialog />);
    fireEvent.input(path(), { target: { value: "~/code/billing-service" } });
    fireEvent.submit(screen.getByRole("dialog"));
    expect(M.addProject).toHaveBeenCalledOnce();
    expect(M.S.newProject).toBeNull();
    expect(M.S.route.pid).toBe("p99");
    expect(M.S.toasts.map((t) => t.msg)).toContain("Project added");
  });

  it("does not add a project without a name", () => {
    const spy = vi.spyOn(M, "addProject");
    render(() => <NewProjectDialog />);
    fireEvent.input(path(), { target: { value: "~/code/x/" } });
    fireEvent.input(name(), { target: { value: "" } });
    expect(add()).toBeDisabled();
    fireEvent.submit(screen.getByRole("dialog"));
    expect(spy).not.toHaveBeenCalled();
  });

  it("closes from Cancel, the X, a scrim click, and Escape", () => {
    const { container } = render(() => <NewProjectDialog />);
    const open = () => M.set({ newProject: { ...EMPTY } });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(M.S.newProject).toBeNull();
    open();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(M.S.newProject).toBeNull();
    open();
    fireEvent.click(container.querySelector(".bg-scrim-dialog") as HTMLElement);
    expect(M.S.newProject).toBeNull();
    open();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(M.S.newProject).toBeNull();
  });

  it("is a bottom sheet on a phone", () => {
    M.setViewport(PHONE_PX, HEIGHT_PX);
    render(() => <NewProjectDialog />);
    expect(screen.getByRole("dialog")).toHaveClass("bottom-0", "rounded-t-xl");
  });
});
