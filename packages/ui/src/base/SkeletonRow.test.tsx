import { render } from "@solidjs/testing-library";
import { SkeletonRow } from "./SkeletonRow";

function shapes(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>("span"));
}

describe("SkeletonRow", () => {
  it("has an avatar, a title bar, and a shorter meta bar", () => {
    const { container } = render(() => <SkeletonRow />);
    const [avatar, title, meta] = shapes(container);
    expect(shapes(container)).toHaveLength(3);
    expect(avatar).toHaveClass("rounded-full");
    expect(avatar).toHaveStyle({ width: "28px", height: "28px" });
    expect(title).toHaveStyle({ width: "60%", height: "12px" });
    expect(meta).toHaveStyle({ width: "35%", height: "10px" });
  });

  it("can lead with a dot", () => {
    const { container } = render(() => <SkeletonRow leading="dot" />);
    expect(shapes(container)[0]).toHaveStyle({ width: "8px", height: "8px" });
  });

  it("is a hidden row that lets the text column shrink, so it never widens the page", () => {
    const { container } = render(() => <SkeletonRow class="px-3" data-testid="row" />);
    const row = container.firstElementChild as HTMLElement;
    expect(row).toHaveAttribute("aria-hidden", "true");
    expect(row).toHaveClass("flex", "items-center", "gap-3", "py-2", "px-3");
    expect(row).toHaveAttribute("data-testid", "row");
    expect(row.querySelector(".min-w-0")).not.toBeNull();
  });
});
