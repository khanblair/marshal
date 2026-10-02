import { cleanup, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { CardHeader } from "./CardHeader";
import { createPanelState } from "./panel-state";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => resetStore());
afterEach(cleanup);

const show = (fields: Partial<ReturnType<typeof cardOf>> = {}, id = "api#35") => {
  Object.assign(cardOf(id), fields);
  const card = cardOf(id);
  return render(() => (
    <CardHeader card={card} c={M.deco(card)} panel={createPanelState()} diffCount={0} />
  ));
};

describe("CardHeader state pill and merge row", () => {
  it("says the generic state for a card the Integrator has no phase for", () => {
    show();
    expect(screen.getByText("Merging")).toBeInTheDocument();
  });

  it("says the phase in plain words in place of the state, with the note under the meta", () => {
    show({ mergePhase: "testing", mergeNote: "Running 14 checks" });
    expect(screen.getByText("Testing the merge")).toBeInTheDocument();
    expect(screen.queryByText("Merging")).toBeNull();
    expect(screen.getByText("Running 14 checks")).toBeInTheDocument();
  });

  it("offers the worktree once the card has one", () => {
    show({ worktree: "/data/worktrees/api/01HZ35" });
    expect(screen.getByRole("button", { name: "Show worktree" })).toBeInTheDocument();
  });

  it("offers no worktree menu before the card starts", () => {
    show();
    expect(screen.queryByRole("button", { name: "Show worktree" })).toBeNull();
  });
});
