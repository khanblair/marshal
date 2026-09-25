import { render } from "@solidjs/testing-library";
import { Kbd, type KbdTone } from "./Kbd";

const kbd = (container: HTMLElement) => container.querySelector("kbd") as HTMLElement;

describe("Kbd", () => {
  it("is a 12 px sans key with a light border by default", () => {
    const { container } = render(() => <Kbd>K</Kbd>);
    expect(kbd(container)).toHaveTextContent("K");
    expect(kbd(container)).toHaveClass(
      "font-sans",
      "text-caption",
      "leading-4",
      "px-1",
      "rounded-xs",
      "border",
      "border-border",
      "text-secondary",
    );
  });

  it.each([
    ["muted", ["border-border", "text-muted"]],
    ["strong", ["border-border-strong", "text-secondary"]],
    ["key", ["border-border-strong", "bg-surface-sunken"]],
    ["current", ["border-current", "opacity-70"]],
  ] as [KbdTone, string[]][])("draws the %s tone", (tone, classes) => {
    const { container } = render(() => <Kbd tone={tone}>A</Kbd>);
    expect(kbd(container)).toHaveClass(...classes);
  });

  it("is 11 px and fainter at the small size", () => {
    const { container } = render(() => (
      <Kbd tone="current" size="sm" class="ml-1">
        P
      </Kbd>
    ));
    expect(kbd(container)).toHaveClass(
      "text-badge",
      "leading-3.5",
      "px-0.75",
      "opacity-60",
      "ml-1",
    );
  });
});
