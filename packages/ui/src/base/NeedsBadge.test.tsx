import { render } from "@solidjs/testing-library";
import { NeedsBadge } from "./NeedsBadge";

describe("NeedsBadge", () => {
  it("shows a raised hand and the count on the needs-you tint", () => {
    const { container } = render(() => <NeedsBadge count={4} title="4 cards need you" />);
    const badge = container.firstElementChild as HTMLElement;
    expect(badge).toHaveTextContent("4");
    expect(badge).toHaveAttribute("title", "4 cards need you");
    expect(badge).toHaveClass(
      "h-5",
      "text-caption",
      "gap-0.75",
      "bg-status-needs-you-subtle",
      "text-status-needs-you-text",
      "font-semibold",
    );
    expect(badge.querySelector("svg")).toHaveAttribute("width", "12");
  });

  it("is 22 px with 13 px text in phone sheets", () => {
    const { container } = render(() => <NeedsBadge count={2} size={22} class="px-1.25!" />);
    expect(container.firstElementChild).toHaveClass("h-5.5", "text-small", "px-1.25!");
  });
});
