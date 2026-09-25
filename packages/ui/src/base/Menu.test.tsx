import { fireEvent, render, screen } from "@solidjs/testing-library";
import { Menu } from "./Menu";
import { MenuItem } from "./MenuItem";
import { MenuLabel } from "./MenuLabel";
import { MenuSeparator } from "./MenuSeparator";

function ProjectMenu(props: { onClose?: () => void; sheet?: boolean }) {
  let trigger: HTMLButtonElement | undefined;
  return (
    <div>
      <button type="button" ref={trigger}>
        More actions
      </button>
      <button type="button">Elsewhere</button>
      <Menu onClose={props.onClose} trigger={() => trigger} sheet={props.sheet} class="w-[200px]">
        <MenuLabel>Project</MenuLabel>
        <MenuItem icon="pencil">Rename</MenuItem>
        <MenuItem icon="settings-2" disabled>
          Project settings
        </MenuItem>
        <MenuSeparator />
        <MenuItem icon="folder-x" danger>
          Remove
        </MenuItem>
      </Menu>
    </div>
  );
}

describe("Menu", () => {
  it("is a raised popover panel with role menu, label, and separator", () => {
    render(() => <ProjectMenu />);
    const menu = screen.getByRole("menu");
    expect(menu).toHaveClass(
      "p-1.5",
      "rounded-lg",
      "border-border",
      "bg-surface-raised",
      "shadow-e1",
      "w-[200px]",
    );
    expect(screen.getByText("Project")).toHaveClass(
      "text-caption",
      "font-semibold",
      "text-secondary",
    );
    expect(menu.querySelector("hr")).toHaveClass("h-px", "my-1.5", "bg-border", "border-none");
    expect(screen.getAllByRole("menuitem")).toHaveLength(3);
  });

  it("closes on a press outside, but not inside or on its trigger", () => {
    const onClose = vi.fn();
    render(() => <ProjectMenu onClose={onClose} />);
    fireEvent.pointerDown(screen.getByRole("menuitem", { name: "Rename" }));
    fireEvent.pointerDown(screen.getByRole("button", { name: "More actions" }));
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.pointerDown(screen.getByRole("button", { name: "Elsewhere" }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("closes on Escape and stops it there", () => {
    const onClose = vi.fn();
    const outer = vi.fn();
    window.addEventListener("keydown", outer);
    render(() => <ProjectMenu onClose={onClose} />);
    fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
    window.removeEventListener("keydown", outer);
    expect(onClose).toHaveBeenCalledOnce();
    expect(outer).not.toHaveBeenCalled();
  });

  it("moves focus between enabled items with the arrow keys, Home, and End", () => {
    render(() => <ProjectMenu />);
    const menu = screen.getByRole("menu");
    const rename = screen.getByRole("menuitem", { name: "Rename" });
    const remove = screen.getByRole("menuitem", { name: "Remove" });
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(rename).toHaveFocus();
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(remove).toHaveFocus();
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(rename).toHaveFocus();
    fireEvent.keyDown(menu, { key: "ArrowUp" });
    expect(remove).toHaveFocus();
    fireEvent.keyDown(menu, { key: "Home" });
    expect(rename).toHaveFocus();
    fireEvent.keyDown(menu, { key: "End" });
    expect(remove).toHaveFocus();
    fireEvent.keyDown(menu, { key: "a" });
    expect(remove).toHaveFocus();
  });

  it("becomes a phone bottom sheet over a scrim, and a scrim press closes it", () => {
    const onClose = vi.fn();
    const { container } = render(() => <ProjectMenu sheet onClose={onClose} />);
    expect(screen.getByRole("menu")).toHaveClass(
      "fixed",
      "bottom-0",
      "rounded-t-xl",
      "max-h-[75%]",
      "shadow-e2",
    );
    const scrim = container.querySelector(".bg-scrim-sheet") as HTMLElement;
    expect(scrim).toHaveClass("fixed", "inset-0");
    fireEvent.pointerDown(scrim);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("does nothing on outside presses without onClose, and chains onKeyDown", () => {
    const onKeyDown = vi.fn();
    render(() => (
      <Menu onKeyDown={onKeyDown}>
        <span>empty</span>
      </Menu>
    ));
    fireEvent.pointerDown(document.body);
    fireEvent.keyDown(screen.getByRole("menu"), { key: "ArrowDown" });
    expect(onKeyDown).toHaveBeenCalledOnce();
  });
});
