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
const EMPTY = { source: "folder", path: "", url: "", name: "", branch: "" } as const;
const IDLE = "Choose a folder or paste a URL. Marshal detects the language and monorepo tools.";

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
    // A folder's branch is read from the repository, so there is no field for it.
    expect(screen.queryByRole("textbox", { name: /branch/i })).toBeNull();
    expect(status()).toHaveTextContent(IDLE);
    expect(add()).toBeDisabled();
  });

  it("fills the name from the folder and never guesses what the repository is", () => {
    render(() => <NewProjectDialog />);
    for (const folder of ["~/code/billing-service", "~/code/my-monorepo", "~/code/website"]) {
      fireEvent.input(path(), { target: { value: folder } });
      expect(status()).toHaveTextContent(IDLE);
    }
    fireEvent.input(path(), { target: { value: "~/code/billing-service" } });
    expect(name()).toHaveValue("billing-service");
    expect(add()).toBeEnabled();
  });

  it("stops following the folder once the name is typed", () => {
    render(() => <NewProjectDialog />);
    fireEvent.input(name(), { target: { value: "Payments" } });
    fireEvent.input(path(), { target: { value: "~/code/other" } });
    expect(name()).toHaveValue("Payments");
    expect(M.S.newProject?.nameTouched).toBe(true);
  });

  it("asks for a branch only when cloning, and leaves it empty for the repository's own", () => {
    render(() => <NewProjectDialog />);
    fireEvent.click(screen.getByRole("radio", { name: "Clone from GitHub" }));
    expect(screen.getByRole("textbox", { name: "Branch" })).toHaveValue("");
    expect(screen.getByRole("textbox", { name: "Branch" })).toHaveAttribute(
      "placeholder",
      "Repository default",
    );
  });

  it("has no Choose folder button: a web page cannot open a folder picker", () => {
    render(() => <NewProjectDialog />);
    expect(screen.queryByRole("button", { name: "Choose folder" })).toBeNull();
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

  it("adds the project, opens its board, and closes", async () => {
    vi.spyOn(M, "addProject").mockResolvedValue({ id: "billing-service" });
    render(() => <NewProjectDialog />);
    fireEvent.input(path(), { target: { value: "~/code/billing-service" } });
    fireEvent.submit(screen.getByRole("dialog"));
    await vi.waitFor(() => expect(M.S.newProject).toBeNull());
    expect(M.addProject).toHaveBeenCalledOnce();
    expect(M.S.route.pid).toBe("billing-service");
    expect(M.S.toasts.map((t) => t.msg)).toContain("Project added");
  });

  it("shows the daemon's refusal under the fields, keeps the dialog open, and keeps the fields", async () => {
    const sentence = "That repository is already a project in Marshal.";
    vi.spyOn(M, "addProject").mockResolvedValue({ error: sentence });
    render(() => <NewProjectDialog />);
    fireEvent.input(path(), { target: { value: "~/code/billing-service" } });
    fireEvent.submit(screen.getByRole("dialog"));
    expect(await screen.findByRole("alert")).toHaveTextContent(sentence);
    expect(screen.getByRole("dialog", { name: "New project" })).toBeInTheDocument();
    expect(path()).toHaveValue("~/code/billing-service");
    expect(name()).toHaveValue("billing-service");
    expect(add()).toBeEnabled();
    expect(M.S.toasts).toHaveLength(0);
  });

  it("drops the daemon's refusal when the source changes, since it was about the other source", async () => {
    vi.spyOn(M, "addProject").mockResolvedValue({ error: "That folder does not exist." });
    render(() => <NewProjectDialog />);
    fireEvent.input(path(), { target: { value: "~/code/missing" } });
    fireEvent.submit(screen.getByRole("dialog"));
    expect(await screen.findByRole("alert")).toHaveTextContent("That folder does not exist.");
    fireEvent.click(screen.getByRole("radio", { name: "Clone from GitHub" }));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("refuses a second submit while the first is still running", async () => {
    let finish: (value: { id: string }) => void = () => {};
    vi.spyOn(M, "addProject").mockReturnValue(new Promise((resolve) => (finish = resolve)));
    render(() => <NewProjectDialog />);
    fireEvent.input(path(), { target: { value: "~/code/billing-service" } });
    fireEvent.submit(screen.getByRole("dialog"));
    expect(add()).toBeDisabled();
    fireEvent.submit(screen.getByRole("dialog"));
    expect(M.addProject).toHaveBeenCalledOnce();
    finish({ id: "billing-service" });
    await vi.waitFor(() => expect(M.S.newProject).toBeNull());
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
