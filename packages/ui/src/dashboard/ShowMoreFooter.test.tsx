import { fireEvent, render, screen } from "@solidjs/testing-library";
import { ShowMoreFooter } from "./ShowMoreFooter";

describe("ShowMoreFooter", () => {
  it("offers Show all N when the list is longer than the limit", () => {
    const onToggle = vi.fn();
    const onViewAll = vi.fn();
    render(() => (
      <ShowMoreFooter
        total={8}
        expanded={false}
        onToggle={onToggle}
        viewAllLabel="View all activity"
        onViewAll={onViewAll}
      />
    ));
    const toggle = screen.getByRole("button", { name: "Show all 8" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(toggle).toHaveClass("border-border-strong", "text-small");
    fireEvent.click(toggle);
    expect(onToggle).toHaveBeenCalledOnce();
    const viewAll = screen.getByRole("button", { name: "View all activity" });
    expect(viewAll).toHaveClass("border-none", "text-secondary", "text-small");
    fireEvent.click(viewAll);
    expect(onViewAll).toHaveBeenCalledOnce();
  });

  it("offers Show less when expanded, and no toggle for short lists", () => {
    const expanded = render(() => (
      <ShowMoreFooter
        total={6}
        expanded
        onToggle={() => {}}
        viewAllLabel="Open agents"
        onViewAll={() => {}}
      />
    ));
    expect(screen.getByRole("button", { name: "Show less" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expanded.unmount();
    render(() => (
      <ShowMoreFooter
        total={5}
        expanded={false}
        onToggle={() => {}}
        viewAllLabel="Open calendar"
        onViewAll={() => {}}
      />
    ));
    expect(screen.getAllByRole("button")).toHaveLength(1);
  });
});
