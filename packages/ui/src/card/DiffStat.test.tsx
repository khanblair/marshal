import { render, screen } from "@solidjs/testing-library";
import { DiffStat } from "./DiffStat";

describe("DiffStat", () => {
  it("shows added and removed counts in the diff colors with a true minus sign", () => {
    const { container } = render(() => <DiffStat added="1,304" removed={12} class="gap-2!" />);
    expect(container.firstElementChild).toHaveClass(
      "font-mono",
      "text-caption",
      "gap-2.5",
      "gap-2!",
    );
    expect(screen.getByText("+1,304")).toHaveClass("text-diff-added-text");
    expect(screen.getByText("−12")).toHaveClass("text-diff-removed-text");
  });
});
