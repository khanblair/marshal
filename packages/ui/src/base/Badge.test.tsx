import { render } from "@solidjs/testing-library";
import { StatusIcon } from "../icons/StatusIcon";
import { Badge, type BadgeTone } from "./Badge";

const first = (container: HTMLElement) => container.firstElementChild as HTMLElement;

describe("Badge", () => {
  it("is a 20 px neutral label with square corners by default", () => {
    const { container } = render(() => <Badge>Not set</Badge>);
    expect(first(container)).toHaveTextContent("Not set");
    expect(first(container)).toHaveClass(
      "h-5",
      "px-1.5",
      "rounded-xs",
      "text-caption",
      "font-semibold",
      "bg-surface-sunken",
      "text-secondary",
    );
  });

  it.each([
    ["working", ["bg-status-working-subtle", "text-status-working-text"]],
    ["needs-you", ["bg-status-needs-you-subtle", "text-status-needs-you-text"]],
    ["danger", ["bg-status-danger-subtle", "text-status-danger-text"]],
    ["selected", ["bg-surface-selected", "text-secondary"]],
    ["outline", ["border", "border-border", "text-secondary"]],
    ["bypass", ["bg-bypass-bg", "text-bypass-text"]],
  ] as [BadgeTone, string[]][])("draws the %s tone", (tone, classes) => {
    const { container } = render(() => <Badge tone={tone}>x</Badge>);
    expect(first(container)).toHaveClass(...classes);
  });

  it.each([
    [18, "h-4.5"],
    [22, "h-5.5"],
    [24, "h-6"],
  ] as const)("sizes to %i px", (size, cls) => {
    const { container } = render(() => <Badge size={size}>x</Badge>);
    expect(first(container)).toHaveClass(cls);
  });

  it("draws a 12 px icon, or a colored state icon passed as a child", () => {
    const { container } = render(() => (
      <Badge tone="review" size={22} icon="check" class="font-mono">
        <StatusIcon state="review" size={12} />
        In review
      </Badge>
    ));
    const icons = container.querySelectorAll("svg");
    expect(icons).toHaveLength(2);
    expect(icons[0]).toHaveAttribute("width", "12");
    expect(icons[1]?.parentElement).toHaveClass("text-status-review-solid");
    expect(first(container)).toHaveClass("font-mono");
  });
});
