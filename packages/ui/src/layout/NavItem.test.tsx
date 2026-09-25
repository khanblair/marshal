import { fireEvent, render, screen } from "@solidjs/testing-library";
import { NeedsBadge } from "../base/NeedsBadge";
import { NavItem } from "./NavItem";

describe("NavItem", () => {
  it("marks the current page with the selected fill and an ink bar", () => {
    const onClick = vi.fn();
    render(() => (
      <NavItem
        icon="house"
        label="Home"
        current
        trailing={<NeedsBadge count={4} />}
        onClick={onClick}
      />
    ));
    const item = screen.getByRole("button", { name: /Home/ });
    expect(item).toHaveAttribute("aria-current", "page");
    expect(item).toHaveAttribute("title", "Home");
    expect(item).toHaveClass("bg-surface-selected", "min-h-8", "px-2", "font-medium", "rounded-sm");
    expect(item.querySelector(".bg-ink")).toHaveClass("absolute", "-left-2", "w-0.5");
    expect(item).toHaveTextContent("Home4");
    fireEvent.click(item);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("is icon-only and centered when collapsed", () => {
    render(() => <NavItem icon="settings" label="Settings" collapsed class="w-full" />);
    const item = screen.getByRole("button", { name: "Settings" });
    expect(item).not.toHaveAttribute("aria-current");
    expect(item).toHaveClass("justify-center", "px-0", "bg-transparent", "w-full");
    expect(item).not.toHaveTextContent("Settings");
    expect(item.querySelector(".bg-ink")).toBeNull();
  });
});
