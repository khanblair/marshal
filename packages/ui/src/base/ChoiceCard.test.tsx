import { fireEvent, render, screen } from "@solidjs/testing-library";
import { ChoiceCard } from "./ChoiceCard";

describe("ChoiceCard", () => {
  it("is a card-shaped radio with a thick ink border when selected", () => {
    const onClick = vi.fn();
    render(() => (
      <div role="radiogroup" aria-label="Theme">
        <ChoiceCard selected class="gap-2.5" onClick={onClick}>
          Light
        </ChoiceCard>
        <ChoiceCard selected={false}>Dark</ChoiceCard>
      </div>
    ));
    const [light, dark] = screen.getAllByRole("radio");
    expect(light).toHaveAttribute("aria-checked", "true");
    expect(light).toHaveAttribute("type", "button");
    expect(light).toHaveClass(
      "border-2",
      "border-ink",
      "rounded-lg",
      "p-3",
      "bg-surface",
      "gap-2.5",
    );
    expect(dark).toHaveAttribute("aria-checked", "false");
    expect(dark).toHaveClass("border", "border-border");
    fireEvent.click(light as HTMLElement);
    expect(onClick).toHaveBeenCalledOnce();
  });
});
