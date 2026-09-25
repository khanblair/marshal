import { render } from "@solidjs/testing-library";
import { Skeleton } from "./Skeleton";

function block(container: HTMLElement): HTMLElement {
  return container.firstElementChild as HTMLElement;
}

describe("Skeleton", () => {
  it("is a decorative rounded block that fills the width and pulses softly", () => {
    const { container } = render(() => <Skeleton />);
    const el = block(container);
    expect(el).toHaveAttribute("aria-hidden", "true");
    expect(el).toHaveClass(
      "block",
      "flex-none",
      "rounded-sm",
      "bg-border",
      "animate-pulse-working",
    );
    expect(el).toHaveStyle({ width: "100%", height: "12px" });
  });

  it("takes its size in px, or as a share of the container", () => {
    const { container: fixed } = render(() => <Skeleton width={120} height={20} />);
    expect(block(fixed)).toHaveStyle({ width: "120px", height: "20px" });
    const { container: share } = render(() => <Skeleton width="60%" />);
    expect(block(share)).toHaveStyle({ width: "60%" });
  });

  it("stays inside a container narrower than its width", () => {
    const { container } = render(() => <Skeleton width={600} />);
    expect(block(container)).toHaveClass("max-w-full");
  });

  it("can be a circle", () => {
    const { container } = render(() => <Skeleton circle width={28} height={28} />);
    expect(block(container)).toHaveClass("rounded-full");
    expect(block(container)).not.toHaveClass("rounded-sm");
  });

  it("adds class, merges style, and passes other attributes to the block", () => {
    const { container } = render(() => (
      <Skeleton class="mt-2" style={{ opacity: 0.5 }} width={40} data-testid="bar" />
    ));
    const el = block(container);
    expect(el).toHaveClass("mt-2", "bg-border");
    expect(el).toHaveStyle({ opacity: "0.5", width: "40px" });
    expect(el).toHaveAttribute("data-testid", "bar");
  });

  it("has no text for a screen reader", () => {
    const { container } = render(() => <Skeleton />);
    expect(container).toHaveTextContent("");
  });
});
