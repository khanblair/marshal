import { fireEvent, render, screen } from "@solidjs/testing-library";
import { SummaryTile } from "./SummaryTile";

describe("SummaryTile", () => {
  it("shows a big number and a label and opens on click", () => {
    const onClick = vi.fn();
    render(() => (
      <SummaryTile
        value={3}
        label="Need you"
        tone="needs-you"
        title="Open the cards that need you"
        onClick={onClick}
      />
    ));
    const tile = screen.getByRole("button", { name: /Need you/ });
    expect(tile).toHaveAttribute("title", "Open the cards that need you");
    expect(tile).toHaveClass(
      "rounded-lg",
      "border-border",
      "py-3.5",
      "px-4",
      "hover:border-border-strong",
    );
    expect(screen.getByText("3")).toHaveClass(
      "text-tile",
      "leading-8.5",
      "font-bold",
      "tabular-nums",
      "text-status-needs-you-text",
    );
    expect(screen.getByText("Need you")).toHaveClass("text-small", "leading-4.5", "text-secondary");
    fireEvent.click(tile);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it.each([
    [undefined, "text-primary"],
    ["danger", "text-status-danger-text"],
  ] as const)("colors the number for tone %s", (tone, cls) => {
    render(() => <SummaryTile value="$4.20" label="Cost today of $25.00" tone={tone} />);
    expect(screen.getByText("$4.20")).toHaveClass(cls);
  });
});
