import { render, screen } from "@solidjs/testing-library";
import { TableCell } from "./TableCell";

describe("TableCell", () => {
  it("draws the line under the cell and takes layout from the class", () => {
    render(() => (
      <table>
        <tbody>
          <tr>
            <TableCell class="px-3 py-2 text-right">$0.84</TableCell>
          </tr>
        </tbody>
      </table>
    ));
    const cell = screen.getByRole("cell", { name: "$0.84" });
    expect(cell).toHaveClass("border-b", "border-border", "px-3", "py-2", "text-right");
  });
});
