import { render } from "@solidjs/testing-library";
import { CountBubble } from "./CountBubble";

describe("CountBubble", () => {
  it("is a 16 px round ink count by default", () => {
    const { container } = render(() => (
      <CountBubble class="absolute top-0.5 right-0">3</CountBubble>
    ));
    const bubble = container.firstElementChild as HTMLElement;
    expect(bubble).toHaveTextContent("3");
    expect(bubble).toHaveClass(
      "min-w-4",
      "h-4",
      "rounded-full",
      "text-badge",
      "font-bold",
      "bg-ink",
      "text-on-ink",
      "absolute",
    );
  });

  it("uses the needs-you solid with dark text", () => {
    const { container } = render(() => <CountBubble tone="needs-you">2</CountBubble>);
    expect(container.firstElementChild).toHaveClass("bg-status-needs-you-solid", "text-on-status");
  });
});
