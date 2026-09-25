import { fireEvent, render, screen } from "@solidjs/testing-library";
import type { JSX } from "solid-js";
import { SortHeader } from "./SortHeader";

function Table(props: { children: JSX.Element }) {
  return (
    <table>
      <thead>
        <tr>{props.children}</tr>
      </thead>
    </table>
  );
}

describe("SortHeader", () => {
  it("is a sticky sort button with an arrow for the active column", () => {
    const onSort = vi.fn();
    render(() => (
      <Table>
        <SortHeader label="State" sort="ascending" onSort={onSort} />
        <SortHeader label="Cost" sort="descending" onSort={() => {}} alignEnd size={34} />
        <SortHeader label="Role" onSort={() => {}} />
      </Table>
    ));
    const [state, cost, role] = screen.getAllByRole("columnheader");
    expect(state).toHaveAttribute("aria-sort", "ascending");
    expect(state).toHaveClass(
      "sticky",
      "top-0",
      "z-sticky",
      "bg-surface-sunken",
      "border-b",
      "h-9",
      "text-left",
    );
    const button = screen.getByRole("button", { name: "State" });
    expect(button).toHaveClass(
      "text-secondary",
      "hover:text-primary",
      "px-3",
      "h-9",
      "justify-start",
    );
    expect(button.querySelector("svg")).toHaveAttribute("width", "12");
    fireEvent.click(button);
    expect(onSort).toHaveBeenCalledOnce();
    expect(cost).toHaveAttribute("aria-sort", "descending");
    expect(cost).toHaveClass("h-8.5", "text-right");
    expect(screen.getByRole("button", { name: "Cost" })).toHaveClass("justify-end");
    expect(role).toHaveAttribute("aria-sort", "none");
    expect(screen.getByRole("button", { name: "Role" }).querySelector("svg")).toBeNull();
  });

  it("is plain text without onSort", () => {
    render(() => (
      <Table>
        <SortHeader label="Actions" />
      </Table>
    ));
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText("Actions")).toHaveClass("block", "px-3", "text-secondary");
  });
});
