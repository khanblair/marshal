import { fireEvent, render, screen } from "@solidjs/testing-library";
import { Avatar } from "./Avatar";
import { MenuItem } from "./MenuItem";

describe("MenuItem", () => {
  it("is a 32 px menuitem row with a 14 px icon", () => {
    const onClick = vi.fn();
    render(() => (
      <MenuItem icon="pencil" onClick={onClick}>
        Rename
      </MenuItem>
    ));
    const item = screen.getByRole("menuitem", { name: "Rename" });
    expect(item).toHaveAttribute("type", "button");
    expect(item).toHaveClass(
      "w-full",
      "h-8",
      "gap-2",
      "px-2",
      "rounded-sm",
      "text-small",
      "text-primary",
      "hover:bg-surface-hover",
    );
    expect(item.querySelector("svg")).toHaveAttribute("width", "14");
    fireEvent.click(item);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("colors danger rows red with a red hover", () => {
    render(() => <MenuItem danger>Delete card</MenuItem>);
    const item = screen.getByRole("menuitem");
    expect(item).toHaveClass("text-status-danger-text", "hover:bg-status-danger-subtle");
    expect(item).not.toHaveClass("text-primary");
  });

  it("draws radio rows with a leading check slot", () => {
    render(() => (
      <>
        <MenuItem kind="radio" checked size={30} hint="By role">
          Claude Code by role
        </MenuItem>
        <MenuItem kind="radio" size={30}>
          All cards
        </MenuItem>
      </>
    ));
    const [on, off] = screen.getAllByRole("menuitemradio");
    expect(on).toHaveAttribute("aria-checked", "true");
    expect(on).toHaveClass("min-h-7.5");
    expect(on?.querySelector("svg")).not.toBeNull();
    expect(screen.getByText("By role")).toHaveClass("text-caption", "text-muted");
    expect(off).toHaveAttribute("aria-checked", "false");
    expect(off?.querySelector("svg")).toBeNull();
  });

  it("draws check rows with a leading avatar and a trailing check", () => {
    render(() => (
      <MenuItem kind="check" checked size={34} leading={<Avatar initials="AO" />}>
        Ada Okafor
      </MenuItem>
    ));
    const item = screen.getByRole("menuitemcheckbox", { name: /Ada Okafor/ });
    expect(item).toHaveAttribute("aria-checked", "true");
    expect(item).toHaveClass("h-8.5");
    expect(item.lastElementChild?.querySelector("svg")).not.toBeNull();
  });

  it("draws plain phone sheet rows with an 18 px icon and the current page", () => {
    render(() => (
      <MenuItem kind="plain" size={48} icon="house" current iconClass="text-muted">
        Home
      </MenuItem>
    ));
    const row = screen.getByRole("button", { name: "Home" });
    expect(row).not.toHaveAttribute("role");
    expect(row).toHaveAttribute("aria-current", "page");
    expect(row).toHaveClass("min-h-12", "gap-3", "rounded-md", "text-lead", "bg-surface-selected");
    expect(row.querySelector("svg")).toHaveAttribute("width", "18");
    expect(row.querySelector("svg")?.parentElement).toHaveClass("text-muted");
  });

  it("uses 16 px icons at 36 px", () => {
    render(() => (
      <MenuItem size={36} icon="user-round">
        Profile
      </MenuItem>
    ));
    const item = screen.getByRole("menuitem");
    expect(item).toHaveClass("min-h-9", "gap-2.5", "px-2.5");
    expect(item.querySelector("svg")).toHaveAttribute("width", "16");
  });
});
