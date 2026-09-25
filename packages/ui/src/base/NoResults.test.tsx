import { fireEvent, render, screen } from "@solidjs/testing-library";
import { NoResults } from "./NoResults";

describe("NoResults", () => {
  it("shows the message with a 28 px clear button", () => {
    const onClear = vi.fn();
    const { container } = render(() => (
      <NoResults onClear={onClear} class="bg-surface">
        No cards match "auth".
      </NoResults>
    ));
    expect(container.firstElementChild).toHaveClass(
      "rounded-md",
      "border",
      "border-border",
      "py-2.5",
      "px-3",
      "bg-surface",
    );
    expect(screen.getByText('No cards match "auth".')).toHaveClass("flex-1");
    const clear = screen.getByRole("button", { name: "Clear search" });
    expect(clear).toHaveClass("h-7");
    fireEvent.click(clear);
    expect(onClear).toHaveBeenCalledOnce();
  });

  it("takes another clear label", () => {
    render(() => (
      <NoResults onClear={() => {}} clearLabel="Clear filters">
        No cards match these filters.
      </NoResults>
    ));
    expect(screen.getByRole("button", { name: "Clear filters" })).toBeInTheDocument();
  });
});
