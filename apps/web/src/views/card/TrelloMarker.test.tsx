import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TrelloView } from "~/data/mappers/trello";
import { M } from "~/mock";
import { platform } from "~/platform";
import { CardMeta } from "./CardMeta";
import { TrelloMarker } from "./TrelloMarker";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

vi.mock("~/platform", () => ({
  platform: vi.fn(() => ({ native: false, openExternal: vi.fn() })),
}));

const URL_41 = "https://trello.com/c/Ab12CdEf";

beforeEach(() => resetStore());
afterEach(() => {
  cleanup();
  vi.mocked(platform).mockReturnValue({ native: false, openExternal: vi.fn() } as never);
  M.set({ trello: {} });
});

/** A linked project whose cards are these, as the daemon (or the mock) names them. */
function link(cardId: string, fromTrello: boolean, listName = "In review"): TrelloView {
  return {
    linked: true,
    boardName: "API",
    changes: [],
    cards: [{ cardId, url: URL_41, listName, fromTrello }],
    fetchedAt: null,
    serverTime: 0,
  };
}

const put = (view: TrelloView) =>
  M.set({ trello: { api: { pending: view, error: "", loading: false } } });

describe("the From Trello marker", () => {
  it("says a card came from Trello, with a link to it and the list it was on", () => {
    const card = cardOf("api#41");
    put(link(card.daemonId ?? card.id, true));
    render(() => <TrelloMarker card={card} />);
    const anchor = screen.getByRole("link", { name: "From Trello" });
    expect(anchor).toHaveAttribute("href", URL_41);
    expect(anchor).toHaveAttribute("target", "_blank");
    expect(anchor).toHaveAttribute("rel", "noopener noreferrer");
    expect(anchor).toHaveAttribute(
      "title",
      expect.stringContaining("starts only when you start it"),
    );
    expect(screen.getByText("List: In review")).toBeInTheDocument();
  });

  it("says On Trello for a Marshal card that was sent there later", () => {
    const card = cardOf("api#41");
    put(link(card.daemonId ?? card.id, false, "Backlog"));
    render(() => <TrelloMarker card={card} />);
    expect(screen.getByRole("link", { name: "On Trello" })).toHaveAttribute("href", URL_41);
    expect(screen.getByText("List: Backlog")).toBeInTheDocument();
    expect(screen.queryByText("From Trello")).toBeNull();
  });

  it("finds a card by its key too, for a store with no daemon", () => {
    const card = cardOf("api#41");
    put(link("api#41", true));
    render(() => <TrelloMarker card={card} />);
    expect(screen.getByRole("link", { name: "From Trello" })).toBeInTheDocument();
  });

  it("draws nothing for a card with no Trello card, or a project that is not linked", () => {
    const card = cardOf("api#41");
    put(link("some-other-card", true));
    render(() => <TrelloMarker card={card} />);
    expect(screen.queryByRole("link")).toBeNull();
    M.set({ trello: {} });
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("opens the system browser on a shell with no new tabs, and leaves the link alone on the web", () => {
    const openExternal = vi.fn();
    vi.mocked(platform).mockReturnValue({ native: true, openExternal } as never);
    const card = cardOf("api#41");
    put(link(card.daemonId ?? card.id, true));
    render(() => <TrelloMarker card={card} />);
    expect(fireEvent.click(screen.getByRole("link", { name: "From Trello" }))).toBe(false);
    expect(openExternal).toHaveBeenCalledWith(URL_41);
  });

  it("shows in the card's header line, next to the branch and the cost", () => {
    const card = cardOf("api#41");
    put(link(card.daemonId ?? card.id, true));
    render(() => <CardMeta card={card} c={M.deco(card)} />);
    expect(screen.getByRole("link", { name: "From Trello" })).toBeInTheDocument();
  });
});
