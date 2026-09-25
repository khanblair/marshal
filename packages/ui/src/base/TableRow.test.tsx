import { fireEvent, render, screen } from "@solidjs/testing-library";
import { TableRow } from "./TableRow";

describe("TableRow", () => {
  it("is a focusable, hoverable row that passes attributes and events through", () => {
    const onClick = vi.fn();
    render(() => (
      <table>
        <tbody>
          <TableRow data-card="41" aria-label="#41 Fix" onClick={onClick}>
            <td>Fix</td>
          </TableRow>
        </tbody>
      </table>
    ));
    const row = screen.getByRole("row", { name: "#41 Fix" });
    expect(row).toHaveAttribute("tabindex", "0");
    expect(row).toHaveAttribute("data-card", "41");
    expect(row).toHaveClass("cursor-pointer", "hover:bg-surface-hover", "bg-transparent");
    expect(row).not.toHaveClass("bg-surface-selected");
    fireEvent.click(row);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("fills the highlighted row with the selected color", () => {
    render(() => (
      <table>
        <tbody>
          <TableRow highlight aria-label="row">
            <td>x</td>
          </TableRow>
        </tbody>
      </table>
    ));
    const row = screen.getByRole("row", { name: "row" });
    expect(row).toHaveClass("bg-surface-selected", "hover:bg-surface-hover");
    expect(row).not.toHaveClass("bg-transparent");
  });
});
