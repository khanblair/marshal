import { fireEvent, render, screen } from "@solidjs/testing-library";
import { createSignal, Show } from "solid-js";
import { Dialog } from "./Dialog";
import { FOCUS_DELAY_MS } from "./focus-trap";

describe("Dialog", () => {
  afterEach(() => vi.useRealTimers());

  it("is a centered modal over a dimmed scrim", () => {
    const onClose = vi.fn();
    const { container } = render(() => (
      <Dialog role="alertdialog" aria-labelledby="t" width={520} onClose={onClose}>
        <h2 id="t">Remove api-gateway</h2>
      </Dialog>
    ));
    const dialog = screen.getByRole("alertdialog", { name: "Remove api-gateway" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog.tagName).toBe("DIV");
    expect(dialog).toHaveClass(
      "fixed",
      "z-dialog",
      "rounded-xl",
      "p-5",
      "gap-3.5",
      "bg-surface-raised",
      "shadow-e2",
      "w-[min(520px,calc(100%-24px))]",
      "-translate-x-1/2",
    );
    const scrim = container.querySelector(".bg-scrim-dialog") as HTMLElement;
    expect(scrim).toHaveClass("z-scrim", "fixed", "inset-0");
    fireEvent.click(scrim);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("is a bottom sheet on phones and a form when it has onSubmit", () => {
    const onSubmit = vi.fn((event: SubmitEvent) => event.preventDefault());
    render(() => (
      <Dialog phone onSubmit={onSubmit} aria-label="New card">
        <button type="submit">Create card</button>
      </Dialog>
    ));
    const dialog = screen.getByRole("dialog", { name: "New card" });
    expect(dialog.tagName).toBe("FORM");
    expect(dialog).toHaveClass("bottom-0", "rounded-t-xl", "max-h-[92%]", "px-4");
    expect(dialog).not.toHaveClass("-translate-x-1/2");
    fireEvent.click(screen.getByRole("button", { name: "Create card" }));
    expect(onSubmit).toHaveBeenCalledOnce();
  });

  it("closes on Escape without letting it reach the app", () => {
    const onClose = vi.fn();
    const outer = vi.fn();
    const onKeyDown = vi.fn();
    window.addEventListener("keydown", outer);
    render(() => <Dialog aria-label="Confirm" onClose={onClose} onKeyDown={onKeyDown} />);
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    window.removeEventListener("keydown", outer);
    expect(onClose).toHaveBeenCalledOnce();
    expect(onKeyDown).toHaveBeenCalledOnce();
    expect(outer).not.toHaveBeenCalled();
  });

  it("keeps Tab inside, focuses data-autofocus on open, and restores focus on close", () => {
    vi.useFakeTimers();
    const [open, setOpen] = createSignal(false);
    render(() => (
      <>
        <button type="button" onClick={() => setOpen(true)}>
          Open
        </button>
        <Show when={open()}>
          <Dialog aria-label="Delete card">
            <button type="button" data-autofocus onClick={() => setOpen(false)}>
              Cancel
            </button>
            <button type="button">Delete card</button>
          </Dialog>
        </Show>
      </>
    ));
    const opener = screen.getByRole("button", { name: "Open" });
    opener.focus();
    fireEvent.click(opener);
    vi.advanceTimersByTime(FOCUS_DELAY_MS);
    const cancel = screen.getByRole("button", { name: "Cancel" });
    const confirm = screen.getByRole("button", { name: "Delete card" });
    expect(cancel).toHaveFocus();
    fireEvent.keyDown(cancel, { key: "Tab", shiftKey: true });
    expect(confirm).toHaveFocus();
    fireEvent.keyDown(confirm, { key: "Tab" });
    expect(cancel).toHaveFocus();
    fireEvent.click(cancel);
    expect(opener).toHaveFocus();
  });

  it("forwards its ref", () => {
    let panel: HTMLElement | undefined;
    render(() => <Dialog aria-label="x" ref={panel} />);
    expect(panel).toBe(screen.getByRole("dialog"));
  });
});
