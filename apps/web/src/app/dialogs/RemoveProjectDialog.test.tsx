import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { RemoveProjectDialog } from "./RemoveProjectDialog";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const seedCards = JSON.parse(JSON.stringify(M.S.cards));
const DESKTOP_PX = 1440;
const PHONE_PX = 390;
const HEIGHT_PX = 900;

const open = (id = "web") => M.set({ removeProject: { id, keepBranches: true, keepMemory: true } });

beforeEach(() => {
  M.setViewport(DESKTOP_PX, HEIGHT_PX);
  M.set({ removeProject: null });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  M.set({ removeProject: null });
  M.S.cards = structuredClone(seedCards);
});

const dialog = () => screen.getByRole("alertdialog");

describe("RemoveProjectDialog", () => {
  it("renders nothing while there is no draft, or the project is gone", () => {
    render(() => <RemoveProjectDialog />);
    expect(screen.queryByRole("alertdialog")).toBeNull();
    open("nope");
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("names the project and says nothing on disk is deleted", () => {
    open();
    render(() => <RemoveProjectDialog />);
    expect(screen.getByRole("alertdialog", { name: "Remove web-dashboard" })).toHaveAttribute(
      "aria-modal",
      "true",
    );
    expect(screen.getByRole("heading", { name: "Remove web-dashboard" })).toHaveAttribute(
      "id",
      "rp-title",
    );
    expect(
      screen.getByText("The repository on disk is never deleted. Marshal only stops managing it."),
    ).toBeInTheDocument();
  });

  it("lists what happens, with counts from the project", () => {
    open();
    render(() => <RemoveProjectDialog />);
    expect(screen.getByText("What happens")).toBeInTheDocument();
    expect(
      screen.getByText(/^Running sessions stop\. \d+ agents are awake now\.$/),
    ).toBeInTheDocument();
    expect(screen.getByText(/^Worktrees are cleaned up\. \d+ worktrees\.$/)).toBeInTheDocument();
    expect(
      screen.getByText(/^Cards and chats are removed from Marshal\. \d+ cards and \d+ chats\.$/),
    ).toBeInTheDocument();
  });

  it("warns about unmerged work and lets you keep the branches", () => {
    open();
    render(() => <RemoveProjectDialog />);
    expect(dialog()).toHaveTextContent(/\d+ cards? (has|have) unmerged work/);
    const keep = screen.getByRole("checkbox", { name: "Keep their branches in the repository" });
    expect(keep).toBeChecked();
    fireEvent.click(keep);
    expect(M.S.removeProject?.keepBranches).toBe(false);
    expect(keep).not.toBeChecked();
  });

  it("leaves the unmerged-work box out when every card is merged", () => {
    for (const c of M.S.cards) if (c.p === "web") c.branch = null;
    open();
    render(() => <RemoveProjectDialog />);
    expect(dialog()).not.toHaveTextContent("unmerged work");
    expect(screen.queryByRole("checkbox", { name: /Keep their branches/ })).toBeNull();
  });

  it("offers to keep the memory folder and shows its path", () => {
    open();
    render(() => <RemoveProjectDialog />);
    const keep = screen.getByRole("checkbox", { name: /Keep the project's memory folder/ });
    expect(keep).toBeChecked();
    expect(screen.getByText("vault/projects/web-dashboard/")).toHaveClass("font-mono");
    fireEvent.click(keep);
    expect(M.S.removeProject?.keepMemory).toBe(false);
  });

  it("removes the project on confirm, in the bypass red", () => {
    const remove = vi.spyOn(M, "removeProject").mockImplementation(() => {});
    open();
    render(() => <RemoveProjectDialog />);
    const confirm = screen.getByRole("button", { name: "Remove project" });
    expect(confirm).toHaveClass("bg-bypass-bg", "text-white");
    fireEvent.click(confirm);
    expect(remove).toHaveBeenCalledWith("web");
    expect(M.S.removeProject).toBeNull();
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("closes without removing from Cancel, a scrim click, and Escape", () => {
    const remove = vi.spyOn(M, "removeProject").mockImplementation(() => {});
    open();
    const { container } = render(() => <RemoveProjectDialog />);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(M.S.removeProject).toBeNull();
    open();
    fireEvent.click(container.querySelector(".bg-scrim-dialog") as HTMLElement);
    expect(M.S.removeProject).toBeNull();
    open();
    fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(M.S.removeProject).toBeNull();
    expect(remove).not.toHaveBeenCalled();
  });

  it("is a 520 px dialog on desktop and a bottom sheet on a phone", () => {
    open();
    render(() => <RemoveProjectDialog />);
    expect(dialog()).toHaveClass("w-[min(520px,calc(100%-24px))]");
    cleanup();
    M.setViewport(PHONE_PX, HEIGHT_PX);
    render(() => <RemoveProjectDialog />);
    expect(within(document.body).getByRole("alertdialog")).toHaveClass("bottom-0", "rounded-t-xl");
  });
});
