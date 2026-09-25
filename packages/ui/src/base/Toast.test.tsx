import { fireEvent, render, screen } from "@solidjs/testing-library";
import { Toast } from "./Toast";
import { ToastRegion } from "./ToastRegion";

describe("Toast", () => {
  it("shows a message on ink with a dismiss button", () => {
    const onDismiss = vi.fn();
    render(() => <Toast onDismiss={onDismiss}>Card started</Toast>);
    const toast = screen.getByRole("status");
    expect(toast).toHaveTextContent("Card started");
    expect(toast).toHaveClass(
      "bg-ink",
      "text-on-ink",
      "rounded-md",
      "min-h-10",
      "max-w-[440px]",
      "pointer-events-auto",
    );
    expect(screen.queryAllByRole("button")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it("shows an action button when given a label", () => {
    const onAction = vi.fn();
    render(() => (
      <Toast actionLabel="Undo" onAction={onAction} onDismiss={() => {}} dismissLabel="Close">
        Chat archived
      </Toast>
    ));
    const undo = screen.getByRole("button", { name: "Undo" });
    expect(undo).toHaveClass("h-7", "border-current", "bg-transparent", "font-semibold");
    fireEvent.click(undo);
    expect(onAction).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument();
  });
});

describe("ToastRegion", () => {
  it("is a polite live region at the bottom, raised above the phone navigation", () => {
    const desktop = render(() => <ToastRegion />);
    expect(desktop.container.firstElementChild).toHaveAttribute("aria-live", "polite");
    expect(desktop.container.firstElementChild).toHaveClass(
      "bottom-4",
      "z-toast",
      "pointer-events-none",
      "items-end",
    );
    const phone = render(() => <ToastRegion phone />);
    expect(phone.container.firstElementChild).toHaveClass(
      "bottom-[calc(72px+env(safe-area-inset-bottom))]",
    );
  });
});
