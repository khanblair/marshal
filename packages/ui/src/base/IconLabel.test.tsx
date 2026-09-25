import { render } from "@solidjs/testing-library";
import { IconLabel } from "./IconLabel";

describe("IconLabel", () => {
  it("puts a 14 px icon before the text with a 4 px gap", () => {
    const { container } = render(() => (
      <IconLabel icon="git-branch" title="Branch">
        marshal/41-fix
      </IconLabel>
    ));
    const label = container.firstElementChild as HTMLElement;
    expect(label).toHaveClass("inline-flex", "items-center", "gap-1");
    expect(label).toHaveTextContent("marshal/41-fix");
    expect(label).toHaveAttribute("title", "Branch");
    expect(label.querySelector("svg")).toHaveAttribute("width", "14");
  });

  it.each([
    [3, "gap-0.75"],
    [6, "gap-1.5"],
  ] as const)("takes a %i px gap", (gap, cls) => {
    const { container } = render(() => (
      <IconLabel icon="pin" gap={gap} size={12} iconClass="text-muted">
        Pinned
      </IconLabel>
    ));
    expect(container.firstElementChild).toHaveClass(cls);
    expect(container.querySelector("svg")).toHaveAttribute("width", "12");
    expect(container.querySelector("svg")?.parentElement).toHaveClass("text-muted");
  });
});
