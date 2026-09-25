import { fireEvent, render, screen } from "@solidjs/testing-library";
import { Tag } from "./Tag";

describe("Tag", () => {
  it("is a 32 px filter chip with a named remove button", () => {
    const onRemove = vi.fn();
    const { container } = render(() => (
      <Tag onRemove={onRemove} removeLabel="Remove filter Status Working">
        <span>Status</span>
        <span>Working</span>
      </Tag>
    ));
    const chip = container.firstElementChild as HTMLElement;
    expect(chip).toHaveTextContent("StatusWorking");
    expect(chip).toHaveClass(
      "h-8",
      "rounded-xs",
      "border-border-strong",
      "bg-surface-sunken",
      "text-small",
      "flex-none",
    );
    const remove = screen.getByRole("button", { name: "Remove filter Status Working" });
    expect(remove).toHaveClass("size-6", "text-secondary");
    fireEvent.click(remove);
    expect(onRemove).toHaveBeenCalledOnce();
  });

  it("is a 28 px attachment chip with an icon", () => {
    const { container } = render(() => (
      <Tag size={28} icon="image" onRemove={() => {}} removeLabel="Remove shot.png">
        shot.png
      </Tag>
    ));
    const chip = container.firstElementChild as HTMLElement;
    expect(chip).toHaveClass("h-7", "rounded-sm", "border-border", "gap-1.5");
    expect(chip.querySelector("svg")).toHaveAttribute("width", "14");
    expect(screen.getByRole("button", { name: "Remove shot.png" })).toHaveClass("size-5.5");
  });
});
