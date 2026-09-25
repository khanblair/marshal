import { fireEvent, render, screen } from "@solidjs/testing-library";
import { Button } from "./Button";

const iconSize = (button: HTMLElement) => button.querySelector("svg")?.getAttribute("width");

describe("Button", () => {
  it("is a secondary 32 px button of type button by default", () => {
    render(() => <Button>Cancel</Button>);
    const button = screen.getByRole("button", { name: "Cancel" });
    expect(button).toHaveAttribute("type", "button");
    expect(button).toHaveClass("h-8", "px-3", "border-border-strong", "bg-surface", "font-medium");
    expect(button).not.toHaveAttribute("data-compact");
  });

  it.each([
    ["primary", ["bg-ink", "text-on-ink", "border-ink", "font-semibold", "hover:bg-ink-hover"]],
    ["quiet", ["border-none", "bg-transparent", "text-secondary", "hover:text-primary"]],
    ["destructive", ["bg-bypass-bg", "border-bypass-bg", "text-white", "font-semibold"]],
  ] as const)("draws the %s variant", (variant, classes) => {
    render(() => <Button variant={variant}>Go</Button>);
    expect(screen.getByRole("button")).toHaveClass(...classes);
  });

  it("uses 13 px text and 14 px icons at 28 px, 16 px icons otherwise", () => {
    render(() => (
      <>
        <Button size={28} icon="play">
          Start
        </Button>
        <Button size={36} icon="plus">
          Add
        </Button>
      </>
    ));
    const [small, large] = screen.getAllByRole("button");
    expect(small).toHaveClass("h-7", "px-2.5", "text-small");
    expect(iconSize(small as HTMLElement)).toBe("14");
    expect(large).toHaveClass("h-9", "px-3.5");
    expect(iconSize(large as HTMLElement)).toBe("16");
  });

  it("takes an icon size", () => {
    render(() => (
      <Button icon="columns-2" iconSize={14}>
        Split view
      </Button>
    ));
    expect(iconSize(screen.getByRole("button"))).toBe("14");
  });

  it("shows a faded key hint on filled buttons and a bordered one on others", () => {
    render(() => (
      <>
        <Button variant="primary" kbd="N">
          New card
        </Button>
        <Button kbd="Esc">Deny</Button>
        <Button size={28} kbd="S">
          Sleep
        </Button>
      </>
    ));
    const [filled, plain, small] = Array.from(document.querySelectorAll("kbd"));
    expect(filled).toHaveTextContent("N");
    expect(filled).toHaveClass("border-current", "opacity-70", "text-caption");
    expect(plain).toHaveClass("border-border-strong", "text-secondary");
    expect(small).toHaveClass("text-badge", "leading-3.5");
  });

  it("colors danger text on secondary and quiet buttons", () => {
    render(() => (
      <>
        <Button tone="danger">Remove device</Button>
        <Button variant="quiet" tone="danger">
          Reject
        </Button>
      </>
    ));
    const [secondary, quiet] = screen.getAllByRole("button");
    expect(secondary).toHaveClass("text-status-danger-text");
    expect(quiet).toHaveClass("text-status-danger-text", "hover:bg-status-danger-subtle");
    expect(quiet).not.toHaveClass("text-secondary");
  });

  it("sets data-compact, passes attributes, and merges classes", () => {
    render(() => (
      <Button compact aria-expanded="true" data-tour="x" class="w-full" type="submit">
        Save
      </Button>
    ));
    const button = screen.getByRole("button");
    expect(button).toHaveAttribute("data-compact", "1");
    expect(button).toHaveAttribute("aria-expanded", "true");
    expect(button).toHaveAttribute("data-tour", "x");
    expect(button).toHaveAttribute("type", "submit");
    expect(button).toHaveClass("w-full", "inline-flex");
  });

  it("clicks, and does not click when disabled", () => {
    const onClick = vi.fn();
    const { unmount } = render(() => <Button onClick={onClick}>Go</Button>);
    fireEvent.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledTimes(1);
    unmount();
    render(() => (
      <Button onClick={onClick} disabled>
        Go
      </Button>
    ));
    const button = screen.getByRole("button");
    expect(button).toBeDisabled();
    expect(button).toHaveClass("disabled:opacity-50");
    fireEvent.click(button);
    expect(onClick).toHaveBeenCalledTimes(1);
  });
});
