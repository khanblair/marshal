import { render } from "@solidjs/testing-library";
import { SkeletonCard } from "./SkeletonCard";

describe("SkeletonCard", () => {
  it("has the box of a board card, so nothing shifts when the real card arrives", () => {
    const { container } = render(() => <SkeletonCard />);
    const card = container.firstElementChild as HTMLElement;
    expect(card).toHaveAttribute("aria-hidden", "true");
    expect(card).toHaveClass(
      "flex",
      "flex-col",
      "gap-2",
      "p-3",
      "bg-surface",
      "border",
      "border-border",
      "border-l-3",
      "rounded-md",
    );
  });

  it("draws a two line title, a state line, and an activity line", () => {
    const { container } = render(() => <SkeletonCard />);
    const bars = Array.from(container.querySelectorAll<HTMLElement>("span"));
    expect(bars).toHaveLength(4);
    expect(bars[1]).toHaveStyle({ width: "60%" });
    expect(bars[2]).toHaveStyle({ width: "40%", height: "10px" });
    expect(bars[3]).toHaveStyle({ width: "65%", height: "10px" });
  });

  it("passes class and attributes on, and has no text", () => {
    const { container } = render(() => <SkeletonCard class="w-72" data-testid="card" />);
    const card = container.firstElementChild as HTMLElement;
    expect(card).toHaveClass("w-72", "p-3");
    expect(card).toHaveAttribute("data-testid", "card");
    expect(container).toHaveTextContent("");
  });
});
