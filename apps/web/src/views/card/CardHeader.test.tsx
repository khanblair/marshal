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
});
afterEach(() => {
  cleanup();
  if (detailsCollapsed()) toggleDetails();
  vi.restoreAllMocks();
});

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
    show({ state: "needs", session: "stopped" });
    fold();
    expect(screen.getByRole("button", { name: /Resume agent/ })).toBeInTheDocument();
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
