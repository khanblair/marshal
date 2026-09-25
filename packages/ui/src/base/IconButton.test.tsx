import { fireEvent, render, screen } from "@solidjs/testing-library";
import { IconButton } from "./IconButton";

describe("IconButton", () => {
  it("is named by its label and draws a 16 px icon in a 28 px ghost square", () => {
    render(() => <IconButton label="Close card" icon="x" />);
    const button = screen.getByRole("button", { name: "Close card" });
    expect(button).toHaveAttribute("type", "button");
    expect(button).toHaveClass(
      "size-7",
      "rounded-sm",
      "border-none",
      "bg-transparent",
      "text-secondary",
    );
    expect(button.querySelector("svg")).toHaveAttribute("width", "16");
  });

  it("uses a 12 px icon and 3 px corners at 24 px", () => {
    render(() => <IconButton label="Remove filter" icon="x" size={24} />);
    const button = screen.getByRole("button");
    expect(button).toHaveClass("size-6", "rounded-xs");
    expect(button.querySelector("svg")).toHaveAttribute("width", "12");
  });

  it.each([
    [32, "size-8"],
    [36, "size-9"],
    [44, "size-11"],
  ] as const)("sizes to %i px", (size, cls) => {
    render(() => <IconButton label="x" icon="bell" size={size} iconSize={18} />);
    const button = screen.getByRole("button");
    expect(button).toHaveClass(cls);
    expect(button.querySelector("svg")).toHaveAttribute("width", "18");
  });

  it("draws outline and primary variants", () => {
    render(() => (
      <>
        <IconButton label="Previous month" icon="chevron-left" variant="outline" />
        <IconButton label="Send message" icon="send-horizontal" variant="primary" disabled />
      </>
    ));
    expect(screen.getByRole("button", { name: "Previous month" })).toHaveClass(
      "border-border-strong",
      "bg-surface",
    );
    const send = screen.getByRole("button", { name: "Send message" });
    expect(send).toHaveClass("bg-ink", "text-on-ink", "disabled:opacity-40");
    expect(send).toBeDisabled();
  });

  it("uses the muted tone for row menus and inherits color for the default tone", () => {
    render(() => (
      <>
        <IconButton label="More" icon="ellipsis-vertical" tone="muted" />
        <IconButton label="Close pane" icon="x" tone="default" />
      </>
    ));
    expect(screen.getByRole("button", { name: "More" })).toHaveClass(
      "text-muted",
      "hover:bg-surface-selected",
      "hover:text-primary",
    );
    const plain = screen.getByRole("button", { name: "Close pane" });
    expect(plain).not.toHaveClass("text-secondary");
    expect(plain).toHaveClass("hover:bg-surface-hover");
  });

  it("sets data-compact, passes attributes, and clicks", () => {
    const onClick = vi.fn();
    render(() => (
      <IconButton
        label="Search"
        icon="search"
        compact
        title="Search"
        onClick={onClick}
        class="ml-1"
      />
    ));
    const button = screen.getByRole("button");
    expect(button).toHaveAttribute("data-compact", "1");
    expect(button).toHaveAttribute("title", "Search");
    expect(button).toHaveClass("ml-1");
    fireEvent.click(button);
    expect(onClick).toHaveBeenCalledOnce();
  });
});
