import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { CardHeader } from "./CardHeader";
import { detailsCollapsed, toggleDetails } from "./details-collapsed";
import { createPanelState } from "./panel-state";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => {
  resetStore();
  // The browser queues a storage event on a timer for each write; the fold is not what these test.
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => undefined);
  vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => undefined);
  setDetails(false, false);
  setDetails(true, true);
});
afterEach(() => {
  cleanup();
  setDetails(false, false);
  setDetails(true, true);
  vi.restoreAllMocks();
});

const show = (fields: Partial<ReturnType<typeof cardOf>> = {}, id = "api#35") => {
  Object.assign(cardOf(id), fields);
  const card = cardOf(id);
  return render(() => (
    <CardHeader card={card} c={M.deco(card)} panel={createPanelState()} diffCount={0} />
  ));
};

function setDetails(mobile: boolean, collapsed: boolean) {
  if (detailsCollapsed(mobile) !== collapsed) toggleDetails(mobile);
}

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

  it("hides a stale merge note when no merge phase is recorded", () => {
    show({ mergePhase: undefined, mergeNote: "Old check result" });
    expect(screen.getByText("Merging")).toBeInTheDocument();
    expect(screen.queryByText("Old check result")).toBeNull();
  });

  it("offers the worktree once the card has one", () => {
    show({ worktree: "/data/worktrees/api/01HZ35" });
    expect(screen.getByRole("button", { name: "Previous workspace" })).toBeInTheDocument();
  });

  it("offers no worktree menu before the card starts", () => {
    show();
    expect(screen.queryByRole("button", { name: "Previous workspace" })).toBeNull();
  });
});

describe("CardHeader folding the details away", () => {
  const fold = () => fireEvent.click(screen.getByRole("button", { name: "Hide card details" }));

  it("hides the worktree, members, actions, and settings, and gives them back", () => {
    show({ worktree: "/data/worktrees/api/01HZ35" });
    expect(screen.getByRole("button", { name: "Previous workspace" })).toBeInTheDocument();
    expect(screen.getByText("Members")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "More actions" })).toBeInTheDocument();

    fold();
    expect(screen.queryByRole("button", { name: "Previous workspace" })).toBeNull();
    expect(screen.queryByText("Members")).toBeNull();
    expect(screen.queryByRole("button", { name: "More actions" })).toBeNull();
    expect(screen.getByRole("tab", { name: /Chat/ })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Show card details" }));
    expect(screen.getByRole("button", { name: "Previous workspace" })).toBeInTheDocument();
  });

  it("keeps the action that leads, so a card that needs an answer is never left without one", () => {
    show({
      state: "needs",
      session: "stopped",
      mergePhase: "testing",
      mergeNote: "Running checks",
    });
    fold();
    expect(screen.getByRole("button", { name: /Resume agent/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "More actions" })).toBeNull();
    expect(screen.getByText("Context")).toBeVisible();
    expect(screen.getByText("Running checks")).toBeVisible();
  });

  it("starts mobile cards folded, then shows and hides every detail above the tabs", () => {
    resetStore(390);
    const card = cardOf("api#35");
    const title = card.title;
    const cardNumber = M.deco(card).num;
    show({
      worktree: "/data/worktrees/api/01HZ35",
      mergeNote: "Running checks",
      mergePhase: "testing",
      state: "needs",
      session: "stopped",
    });

    const showDetails = screen.getByRole("button", { name: "Show details" });
    expect(showDetails).toHaveAttribute("aria-expanded", "false");
    expect(showDetails).toHaveAttribute("aria-controls", "card-details");
    expect(showDetails).toHaveClass("min-h-11!");
    expect(screen.getByRole("tab", { name: /Chat/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Back" })).toBeInTheDocument();
    expect(screen.getByText(title)).toBeVisible();
    expect(screen.getByText(cardNumber)).toBeVisible();
    expect(screen.queryByText("Context")).toBeNull();
    expect(screen.queryByText("Running checks")).toBeNull();
    expect(screen.queryByText("Members")).toBeNull();
    expect(screen.queryByRole("button", { name: "Previous workspace" })).toBeNull();
    expect(screen.queryByRole("button", { name: "More actions" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Resume agent/ })).toBeNull();
    expect(document.querySelector('[aria-controls="card-agent-settings"]')).toBeNull();

    fireEvent.click(showDetails);
    expect(screen.getByRole("button", { name: "Hide details" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(screen.getByText("Context")).toBeVisible();
    expect(screen.getByText("Running checks")).toBeVisible();
    expect(screen.getByText("Members")).toBeVisible();
    expect(screen.getByRole("button", { name: "Previous workspace" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "More actions" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Resume agent/ })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Hide details" }));
    expect(screen.getByRole("tab", { name: /Chat/ })).toBeInTheDocument();
    expect(screen.queryByText("Context")).toBeNull();
    expect(screen.queryByRole("button", { name: "More actions" })).toBeNull();
  });

  it("puts the expanded settings' hide control beside their heading", () => {
    const panel = createPanelState();
    panel.set({ settingsExpanded: true });
    const card = cardOf("api#35");
    render(() => <CardHeader card={card} c={M.deco(card)} panel={panel} diffCount={0} />);

    expect(screen.getByText("Agent settings")).toBeInTheDocument();
    const hide = screen.getByRole("button", { name: "Hide settings" });
    expect(hide).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(hide);
    expect(screen.queryByRole("button", { name: "Hide settings" })).toBeNull();
  });
});
