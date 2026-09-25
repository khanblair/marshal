import { fireEvent, render, screen } from "@solidjs/testing-library";
import { MenuItem } from "../base/MenuItem";
import { Sheet } from "./Sheet";

describe("Sheet", () => {
  it("is a titled phone bottom sheet with a grab handle over a scrim", () => {
    const onClose = vi.fn();
    const { container } = render(() => (
      <Sheet title="Go to" onClose={onClose}>
        <MenuItem kind="plain" size={48} icon="house">
          Home
        </MenuItem>
      </Sheet>
    ));
    const sheet = screen.getByRole("dialog", { name: "Go to" });
    expect(sheet).toHaveAttribute("aria-modal", "true");
    expect(sheet).toHaveClass(
      "fixed",
      "bottom-0",
      "z-sheet",
      "max-h-[85%]",
      "rounded-t-xl",
      "bg-surface-raised",
      "px-3",
    );
    expect(screen.getByRole("heading", { name: "Go to" })).toHaveClass(
      "text-subtitle",
      "font-semibold",
    );
    expect(sheet.querySelector(".bg-border-strong")).toHaveClass("w-9", "h-1", "rounded-full");
    const scrim = container.querySelector(".bg-scrim-sheet") as HTMLElement;
    fireEvent.click(scrim);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("can skip the scrim, title, and handle", () => {
    const { container } = render(() => <Sheet aria-label="Tour">Step 1 of 9</Sheet>);
    expect(container.querySelector(".bg-scrim-sheet")).toBeNull();
    expect(screen.queryByRole("heading")).toBeNull();
    expect(
      screen.getByRole("dialog", { name: "Tour" }).querySelector(".bg-border-strong"),
    ).toBeNull();
  });
});
