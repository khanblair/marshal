// First, so the store `~/mock` builds is the one that follows a fake daemon (S5a is the daemon's).
import { daemon, resetDaemonCards, resetStoreCards } from "~/testing/daemon-cards-store";
import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { ConfirmDialog } from "./ConfirmDialog";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const DESKTOP_PX = 1440;
const PHONE_PX = 390;
const HEIGHT_PX = 900;
const FOCUS_MS = 20;

beforeEach(() => {
  vi.useFakeTimers();
  // The card a test deletes is really removed from the daemon, so it is put back for the next one.
  resetDaemonCards();
  M.setViewport(DESKTOP_PX, HEIGHT_PX);
  M.set({ dialog: null, toasts: [] });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  M.set({ dialog: null, toasts: [] });
  resetStoreCards();
});

const dialog = () => screen.getByRole("alertdialog");

function ask(overrides: Partial<Parameters<typeof M.confirm>[0]> = {}) {
  const run = vi.fn();
  M.confirm({
    title: "Sleep all",
    message: "Every idle card sleeps.",
    action: "Sleep all",
    run,
    ...overrides,
  });
  return run;
}

describe("ConfirmDialog", () => {
  it("renders nothing while there is no dialog", () => {
    render(() => <ConfirmDialog />);
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("names itself by its title and shows the message", () => {
    ask();
    render(() => <ConfirmDialog />);
    expect(screen.getByRole("alertdialog", { name: "Sleep all" })).toHaveAttribute(
      "aria-modal",
      "true",
    );
    expect(screen.getByRole("heading", { name: "Sleep all" })).toHaveAttribute("id", "dlg-title");
    expect(screen.getByText("Every idle card sleeps.")).toBeInTheDocument();
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("puts focus on Cancel once it is painted", () => {
    ask();
    render(() => <ConfirmDialog />);
    vi.advanceTimersByTime(FOCUS_MS);
    expect(screen.getByRole("button", { name: "Cancel" })).toHaveFocus();
  });

  it("runs the action and closes when confirmed", () => {
    const run = ask();
    render(() => <ConfirmDialog />);
    const confirm = screen.getByRole("button", { name: "Sleep all" });
    expect(confirm).toHaveClass("bg-ink", "text-on-ink");
    fireEvent.click(confirm);
    expect(run).toHaveBeenCalledOnce();
    expect(M.S.dialog).toBeNull();
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("closes without running on Cancel, on a scrim click, and on Escape", () => {
    const run = ask();
    const { container } = render(() => <ConfirmDialog />);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(M.S.dialog).toBeNull();
    ask();
    fireEvent.click(container.querySelector(".bg-scrim-dialog") as HTMLElement);
    expect(M.S.dialog).toBeNull();
    ask();
    fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(M.S.dialog).toBeNull();
    expect(run).not.toHaveBeenCalled();
  });

  it("styles a destructive action in the bypass red", () => {
    ask({ destructive: true, action: "Delete card" });
    render(() => <ConfirmDialog />);
    expect(screen.getByRole("button", { name: "Delete card" })).toHaveClass(
      "bg-bypass-bg",
      "text-white",
    );
  });

  it("keeps the action disabled until the acknowledgement is checked", () => {
    const run = ask({ ack: "I understand.", destructive: true, action: "Turn on bypass" });
    render(() => <ConfirmDialog />);
    const action = screen.getByRole("button", { name: "Turn on bypass" });
    const box = screen.getByRole("checkbox", { name: "I understand." });
    expect(box).not.toBeChecked();
    expect(action).toBeDisabled();
    fireEvent.click(action);
    expect(run).not.toHaveBeenCalled();
    fireEvent.click(box);
    expect(M.S.dialog?.acked).toBe(true);
    expect(action).toBeEnabled();
    fireEvent.click(action);
    expect(run).toHaveBeenCalledOnce();
    expect(M.S.dialog).toBeNull();
  });

  it("is the bypass request the card settings open", () => {
    M.requestBypass("api#41");
    render(() => <ConfirmDialog />);
    expect(screen.getByRole("heading", { name: "Turn on bypass permissions" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Turn on bypass" })).toBeDisabled();
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "Turn on bypass" }));
    expect(M.card("api#41")?.bypass).toBe(true);
    expect(M.S.toasts.map((t) => t.msg)).toContain("Bypass turned on");
  });

  it("is the delete confirmation for a card", async () => {
    M.deleteCard("api#41");
    render(() => <ConfirmDialog />);
    expect(screen.getByRole("heading", { name: "Delete card" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Delete card" }));
    // The daemon is asked, so the card leaves the store once its answer is in, and the daemon too.
    await vi.waitFor(() => expect(M.card("api#41")).toBeUndefined());
    expect(daemon.cards.find((card) => card.key === "api#41")).toBeUndefined();
  });

  it("is a bottom sheet on a phone", () => {
    M.setViewport(PHONE_PX, HEIGHT_PX);
    ask();
    render(() => <ConfirmDialog />);
    expect(dialog()).toHaveClass("bottom-0", "rounded-t-xl");
  });

  it("keeps Tab inside the dialog", () => {
    ask();
    render(() => <ConfirmDialog />);
    const action = screen.getByRole("button", { name: "Sleep all" });
    action.focus();
    fireEvent.keyDown(action, { key: "Tab" });
    expect(screen.getByRole("button", { name: "Cancel" })).toHaveFocus();
  });
});
