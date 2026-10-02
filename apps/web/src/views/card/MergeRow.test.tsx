import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { openCardWorktree, retryMerge } from "~/sync/integration-flow";
import { MergeRow } from "./MergeRow";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});
vi.mock("~/sync/integration-flow", () => ({
  openCardWorktree: vi.fn(async () => true),
  retryMerge: vi.fn(async () => true),
  undoMerge: vi.fn(async () => true),
}));

const PATH = "/data/worktrees/api/01HZ41";
const PHONE_PX = 390;
const writeText = vi.fn(async () => undefined);

beforeEach(() => {
  resetStore();
  vi.clearAllMocks();
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
});
afterEach(cleanup);

/** Renders the row for a card after the daemon's fields on it were set. */
const show = (fields: Partial<ReturnType<typeof cardOf>> = {}, id = "api#43") => {
  Object.assign(cardOf(id), fields);
  const card = cardOf(id);
  return render(() => <MergeRow card={card} c={M.deco(card)} />);
};
const openMenu = () => fireEvent.click(screen.getByRole("button", { name: "Show worktree" }));

describe("the worktree menu", () => {
  it("is not drawn for a card that has no worktree", () => {
    show();
    expect(screen.queryByRole("button", { name: "Show worktree" })).toBeNull();
  });

  it("opens with three choices", () => {
    show({ worktree: PATH });
    expect(screen.queryByRole("menu")).toBeNull();
    openMenu();
    expect(screen.getAllByRole("menuitem").map((item) => item.textContent)).toEqual([
      "Copy path",
      "Reveal in Finder",
      "Open in editor",
    ]);
  });

  it("opens as a bottom sheet on a phone, and as a popover otherwise", () => {
    show({ worktree: PATH });
    openMenu();
    expect(screen.getByRole("menu")).toHaveClass("absolute");
    cleanup();
    resetStore(PHONE_PX);
    show({ worktree: PATH });
    openMenu();
    expect(screen.getByRole("menu")).toHaveClass("fixed");
    expect(screen.getByRole("menu")).not.toHaveClass("absolute");
  });

  it("copies the path to the clipboard and says so", () => {
    show({ worktree: PATH });
    openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Copy path" }));
    expect(writeText).toHaveBeenCalledWith(PATH);
    expect(M.S.toasts.at(-1)?.msg).toBe("Worktree path copied");
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("reveals the folder in Finder, by the card's key", () => {
    show({ worktree: PATH });
    openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Reveal in Finder" }));
    expect(openCardWorktree).toHaveBeenCalledWith("api#43", "finder");
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("opens the folder in the editor", () => {
    show({ worktree: PATH });
    openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Open in editor" }));
    expect(openCardWorktree).toHaveBeenCalledWith("api#43", "editor");
  });
});

describe("Retry merge", () => {
  it("is offered when the merge queue sent the card to Needs you", () => {
    show({ state: "needs", reasonKind: "conflict", reason: "api.go conflicts with development." });
    fireEvent.click(screen.getByRole("button", { name: "Retry merge" }));
    expect(retryMerge).toHaveBeenCalledWith("api#43");
  });

  it("is offered for a merge queue's failed tests, and not for the pull request's own CI", () => {
    show({
      state: "needs",
      reasonKind: "ci-failed",
      reason: "The merge queue's tests did not pass: 2 failed.",
    });
    expect(screen.getByRole("button", { name: "Retry merge" })).toBeInTheDocument();
    cleanup();
    show({
      state: "needs",
      reasonKind: "ci-failed",
      reason: "CI is still failing on this branch.",
    });
    expect(screen.queryByRole("button", { name: "Retry merge" })).toBeNull();
  });

  it("is not offered for a card that waits for another reason", () => {
    show({ state: "needs", reasonKind: "question", reason: "Which branch?" });
    expect(screen.queryByRole("button", { name: "Retry merge" })).toBeNull();
  });
});

describe("the merge note", () => {
  it("shows the Integrator's sentence while a merge has a phase", () => {
    show({ state: "merging", mergePhase: "resolving", mergeNote: "Resolving 2 conflicts" });
    expect(screen.getByText("Resolving 2 conflicts")).toBeInTheDocument();
  });

  it("shows nothing for a note without a phase", () => {
    show({ mergeNote: "Left over" });
    expect(screen.queryByText("Left over")).toBeNull();
  });
});
