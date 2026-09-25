import { render, screen } from "@solidjs/testing-library";
import { For } from "solid-js";
import { SkeletonCard } from "./SkeletonCard";
import { SkeletonGroup } from "./SkeletonGroup";
import { SkeletonRow } from "./SkeletonRow";

describe("SkeletonGroup", () => {
  it("is one status with one visually hidden Loading", () => {
    render(() => (
      <SkeletonGroup>
        <SkeletonRow />
      </SkeletonGroup>
    ));
    const status = screen.getByRole("status");
    const label = screen.getByText("Loading");
    expect(status).toContainElement(label);
    expect(label).toHaveClass("sr-only");
  });

  it("announces once for twenty rows and a card, and hides every shape", () => {
    const { container } = render(() => (
      <div aria-busy="true">
        <SkeletonGroup>
          <For each={Array.from({ length: 20 }, (_, i) => i)}>{() => <SkeletonRow />}</For>
          <SkeletonCard />
        </SkeletonGroup>
      </div>
    ));
    expect(screen.getAllByRole("status")).toHaveLength(1);
    expect(screen.getAllByText("Loading")).toHaveLength(1);
    const shapes = container.querySelectorAll("span:not(.sr-only)");
    expect(shapes.length).toBeGreaterThan(20);
    for (const shape of shapes) expect(shape.closest("[aria-hidden='true']")).not.toBeNull();
    expect(container.querySelector("[aria-busy='true']")).not.toBeNull();
  });

  it("takes another label, and adds class and attributes", () => {
    render(() => <SkeletonGroup label="Loading cards" class="flex flex-col" data-testid="group" />);
    expect(screen.getByText("Loading cards")).toHaveClass("sr-only");
    expect(screen.queryByText("Loading")).toBeNull();
    const group = screen.getByRole("status");
    expect(group).toHaveClass("flex", "flex-col");
    expect(group).toHaveAttribute("data-testid", "group");
  });

  it("does not mark itself busy: that is for the container the caller is loading", () => {
    render(() => <SkeletonGroup />);
    expect(screen.getByRole("status")).not.toHaveAttribute("aria-busy");
  });
});
