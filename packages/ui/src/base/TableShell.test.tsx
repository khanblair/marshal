import { render, screen } from "@solidjs/testing-library";
import { SortHeader } from "./SortHeader";
import { TableShell } from "./TableShell";

describe("TableShell", () => {
  it("puts the header cells in one row above the body rows", () => {
    render(() => (
      <TableShell
        minWidth={820}
        header={
          <>
            <SortHeader label="Role" />
            <SortHeader label="Agent" />
          </>
        }
      >
        <tr>
          <td>Worker</td>
        </tr>
      </TableShell>
    ));
    const table = screen.getByRole("table");
    expect(table).toHaveClass("w-full", "border-separate", "border-spacing-0", "text-small");
    expect(table).toHaveClass("leading-4.5");
    expect(table).toHaveStyle({ "min-width": "820px" });
    expect(table.querySelectorAll("thead tr")).toHaveLength(1);
    expect(screen.getAllByRole("columnheader")).toHaveLength(2);
    expect(table.querySelector("tbody")).toHaveTextContent("Worker");
  });
});
