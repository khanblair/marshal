import { render } from "@solidjs/testing-library";
import { SkeletonLines } from "./SkeletonLines";

function bars(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>("span"));
}

describe("SkeletonLines", () => {
  it("draws three lines by default, the last one shorter", () => {
    const { container } = render(() => <SkeletonLines />);
    const lines = bars(container);
    expect(lines).toHaveLength(3);
    expect(lines[0]).toHaveStyle({ width: "100%", height: "12px" });
    expect(lines[1]).toHaveStyle({ width: "100%" });
    expect(lines[2]).toHaveStyle({ width: "60%" });
  });

  it("draws the number of lines asked for", () => {
    const { container } = render(() => <SkeletonLines lines={5} lineHeight={16} />);
    const lines = bars(container);
    expect(lines).toHaveLength(5);
    expect(lines[4]).toHaveStyle({ width: "60%", height: "16px" });
  });

  it("always draws at least one line", () => {
    const { container: none } = render(() => <SkeletonLines lines={0} />);
    expect(bars(none)).toHaveLength(1);
    const { container: fraction } = render(() => <SkeletonLines lines={2.9} />);
    expect(bars(fraction)).toHaveLength(2);
  });

  it("hides the group from a screen reader and passes class and attributes on", () => {
    const { container } = render(() => <SkeletonLines class="max-w-60" data-testid="text" />);
    const group = container.firstElementChild as HTMLElement;
    expect(group).toHaveAttribute("aria-hidden", "true");
    expect(group).toHaveClass("flex", "flex-col", "gap-2", "max-w-60");
    expect(group).toHaveAttribute("data-testid", "text");
  });
});
