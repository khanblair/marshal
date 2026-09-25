import { fireEvent, render, screen, within } from "@solidjs/testing-library";
import { describe, expect, it } from "vitest";
import { M } from "~/mock";
import { BoardView } from "./BoardView";
import { column, useBoardTestStore } from "./board-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

useBoardTestStore();

describe("BoardView swimlanes", () => {
  it("draws a lane header per role with its card count", () => {
    M.S.swim.api = "role";
    render(() => <BoardView />);
    const headers = screen.getAllByRole("button", { expanded: true });
    expect(headers.map((h) => h.textContent)).toEqual([
      "Docs writer1 card",
      "Integrator1 card",
      "Tester1 card",
      "Worker8 cards",
    ]);
    expect(document.querySelectorAll("section[data-col]")).toHaveLength(4 * 7);
    expect(within(column("backlog")).queryByText("No cards")).not.toBeInTheDocument();
  });

  it("uses monospace for package lane names and puts No package last", () => {
    M.go("project", "mobile", "board");
    render(() => <BoardView />);
    const names = screen
      .getAllByRole("button", { expanded: true })
      .map((h) => h.firstElementChild?.nextElementSibling);
    expect(names.map((el) => el?.textContent)).toEqual([
      "apps/ios",
      "apps/android",
      "packages/ui",
      "packages/auth",
      "packages/api-client",
    ]);
    expect(names[0]).toHaveClass("font-mono", "text-small");
  });

  it("collapses and expands a lane, remembering it per project and swimlane mode", () => {
    M.S.swim.api = "role";
    render(() => <BoardView />);
    const worker = screen.getByRole("button", { name: /^Worker/ });
    fireEvent.click(worker);
    expect(worker).toHaveAttribute("aria-expanded", "false");
    expect(M.S.laneCollapsed["api:role:Worker"]).toBe(true);
    expect(document.querySelectorAll("section[data-col]")).toHaveLength(3 * 7);
    expect(M.nav?.grid?.flat()).not.toContain(41);
    fireEvent.click(worker);
    expect(worker).toHaveAttribute("aria-expanded", "true");
    expect(document.querySelectorAll("section[data-col]")).toHaveLength(4 * 7);
  });
});
