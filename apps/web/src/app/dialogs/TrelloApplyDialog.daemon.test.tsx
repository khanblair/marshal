// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (S5a is the daemon's).
import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { daemon, resetDaemonCards, resetStoreCards } from "~/testing/daemon-cards-store";
import { M } from "~/mock";
import { linkedTrello, trelloChange } from "~/testing/fake-trello";
import { CardMeta } from "~/views/card/CardMeta";
import { TrelloBar } from "~/views/trello/TrelloBar";
import { TrelloApplyDialog } from "./TrelloApplyDialog";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * The Apply to Trello flow against a store that follows the fake daemon: the pending changes are
 * read when a project is shown and again on `trello.changed`, every action is the daemon's route
 * with the ids the person picked, and what the daemon writes or refuses is what the report says.
 */
const READ = "GET /v1/projects/api/trello/pending";
const APPLY = "POST /v1/projects/api/trello/apply";
const DISMISS = "POST /v1/projects/api/trello/dismiss";
const REFRESH = "POST /v1/projects/api/trello/refresh";
const URL_41 = "https://trello.com/c/Ab12CdEf";

const idOf = (key: string): string => {
  const card = daemon.cards.find((one) => one.key === key);
  if (!card) throw new Error(`the fake daemon has no card ${key}`);
  return card.id;
};
const count = (route: string): number => daemon.routes().filter((one) => one === route).length;

function seed(): void {
  const card = idOf("api#41");
  daemon.trello.states.api = linkedTrello(
    [
      trelloChange({
        id: "c-move",
        cardId: card,
        cardTitle: "Fix token refresh on login",
        kind: "moveDone",
        summary: "Move to Done",
        byKind: "person",
        conflict: true,
        conflictNote: "Trello moved this card to Review.",
      }),
      trelloChange({
        id: "c-comment",
        cardId: card,
        cardTitle: "Fix token refresh on login",
        summary: "Add a comment",
        detail: "Refresh tokens are now rotated on every login.",
      }),
      trelloChange({
        id: "c-desc",
        cardId: idOf("api#42"),
        cardTitle: "Rate limit the public API",
        kind: "updateCard",
        summary: "Change the description",
        byKind: "person",
      }),
    ],
    [{ cardId: card, url: URL_41, listName: "In review", fromTrello: true }],
  );
}

/** Shows the project's board, which is what makes the store read its Trello changes. */
function show() {
  M.go("project", "api", "board");
  return render(() => (
    <>
      <TrelloBar pid="api" />
      <TrelloApplyDialog />
    </>
  ));
}

const press = (name: RegExp | string): void => {
  fireEvent.click(screen.getByRole("button", { name }));
};
const openDialog = async () => {
  fireEvent.click(await screen.findByRole("button", { name: /Apply to Trello/ }));
  return await screen.findByRole("dialog", { name: "Apply to Trello" });
};

beforeEach(() => {
  delete daemon.trello.states.api;
  daemon.trello.written.length = 0;
  daemon.trello.kept.length = 0;
  daemon.trello.refused = {};
  daemon.trello.imports = 0;
  resetDaemonCards();
  resetStoreCards();
  // Away from the project first, so showing it is a change the store follows.
  M.go("project", "mobile", "list");
  M.set({ trello: {}, trelloApply: null, toasts: [] });
  daemon.calls.length = 0;
});
afterEach(cleanup);

describe("reading the changes of the project that is shown", () => {
  it("reads them once when the board is shown, and draws the strip with the count", async () => {
    seed();
    show();
    const apply = await screen.findByRole("button", { name: /Apply to Trello/ });
    expect(apply).toHaveTextContent("3");
    expect(screen.getByText("Mobile app")).toBeInTheDocument();
    expect(count(READ)).toBe(1);
  });

  it("draws nothing for a project that is not linked, and reads once", async () => {
    show();
    await vi.waitFor(() => expect(M.S.trello?.api?.pending?.linked).toBe(false));
    expect(count(READ)).toBe(1);
    expect(document.querySelector("[data-trello-bar]")).toBeNull();
  });

  it("reads again when the daemon says the changes changed", async () => {
    seed();
    show();
    await screen.findByRole("button", { name: /Apply to Trello/ });
    daemon.trello.states.api?.changes.push(
      trelloChange({ id: "c-new", cardId: idOf("api#42"), summary: "Add a comment" }),
    );
    daemon.emit("project:api", "trello.changed", { projectId: "api" });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /Apply to Trello/ })).toHaveTextContent("4"),
    );
    expect(count(READ)).toBe(2);
  });

  it("shows the sentence of a failed read and keeps what it had", async () => {
    seed();
    show();
    await screen.findByRole("button", { name: /Apply to Trello/ });
    daemon.refuseNext(READ, 503, "unavailable", "Trello did not answer. Try again.");
    daemon.emit("project:api", "trello.changed", { projectId: "api" });
    await waitFor(() => expect(M.S.trello?.api?.error).toBe("Trello did not answer. Try again."));
    expect(M.S.trello?.api?.pending?.changes).toHaveLength(3);
    expect(await screen.findByText("Trello did not answer. Try again.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(M.S.trello?.api?.error).toBe(""));
    expect(screen.queryByText("Trello did not answer. Try again.")).toBeNull();
  });
});

describe("applying through the daemon", () => {
  it("sends the picked ids, writes them, and says what was written", async () => {
    seed();
    show();
    await openDialog();
    press("Apply 2 changes");
    const written = await screen.findByRole("region", { name: "Written to Trello" });
    expect(within(written).getByRole("heading")).toHaveTextContent("Written to Trello (2)");
    expect(daemon.bodies(APPLY)).toEqual([{ changeIds: ["c-comment", "c-desc"], overrideIds: [] }]);
    expect(daemon.trello.written).toEqual(["c-comment", "c-desc"]);
    expect(M.S.trello?.api?.pending?.changes.map((c) => c.id)).toEqual(["c-move"]);
  });

  it("sends the override of a conflict that was applied anyway", async () => {
    seed();
    show();
    await openDialog();
    fireEvent.click(screen.getByRole("button", { name: "Apply anyway" }));
    press("Apply 3 changes");
    await screen.findByRole("region", { name: "Written to Trello" });
    expect(daemon.bodies(APPLY)).toEqual([
      { changeIds: ["c-move", "c-comment", "c-desc"], overrideIds: ["c-move"] },
    ]);
    expect(daemon.trello.written).toContain("c-move");
  });

  it("says why a change was not written, in the daemon's words, and keeps it pending", async () => {
    seed();
    daemon.trello.refused["c-comment"] = "The Trello card was deleted.";
    show();
    await openDialog();
    press("Apply 2 changes");
    const left = await screen.findByRole("region", { name: "Not written" });
    expect(within(left).getByText("Add a comment")).toBeInTheDocument();
    expect(within(left).getByText("The Trello card was deleted.")).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Written to Trello" })).toBeInTheDocument();
    expect(M.S.trello?.api?.pending?.changes.map((c) => c.id)).toEqual(["c-move", "c-comment"]);
  });

  it("sends Skip as a dismiss of that change, and Keep Trello's version as a refresh of its card", async () => {
    seed();
    show();
    await openDialog();
    const row = screen.getByRole("checkbox", { name: /Move to Done/ }).closest("li") as HTMLElement;
    fireEvent.click(within(row).getByRole("button", { name: "Keep Trello's version" }));
    await waitFor(() => expect(daemon.bodies(REFRESH)).toHaveLength(1));
    expect(daemon.bodies(REFRESH)[0]).toEqual({ keepTrelloCardIds: [idOf("api#41")] });
    // Each answer draws the list again, so the row is looked up again.
    await screen.findByText(/Kept Trello's version of this card's title/);
    const again = screen
      .getByRole("checkbox", { name: /Move to Done/ })
      .closest("li") as HTMLElement;
    fireEvent.click(within(again).getByRole("button", { name: "Skip" }));
    await waitFor(() => expect(daemon.bodies(DISMISS)).toHaveLength(1));
    expect(daemon.bodies(DISMISS)[0]).toEqual({ changeIds: ["c-move"] });
    await waitFor(() =>
      expect(screen.queryByRole("checkbox", { name: /Move to Done/ })).toBeNull(),
    );
  });

  it("a refused apply leaves the list as it was, and shows the daemon's sentence", async () => {
    seed();
    show();
    await openDialog();
    daemon.refuseNext(
      APPLY,
      409,
      "conflict",
      "Trello changed while you chose. Nothing was written.",
    );
    press("Apply 2 changes");
    await waitFor(() =>
      expect(M.S.toasts.map((t) => t.msg)).toContain(
        "Trello changed while you chose. Nothing was written.",
      ),
    );
    expect(screen.getByRole("button", { name: "Apply 2 changes" })).toBeEnabled();
    expect(M.S.trello?.api?.pending?.changes).toHaveLength(3);
  });
});

describe("Refresh from Trello", () => {
  it("reads the board with nothing set back, and says how many cards it made and updated", async () => {
    seed();
    daemon.trello.imports = 2;
    show();
    await screen.findByRole("button", { name: /Apply to Trello/ });
    press("Refresh from Trello");
    await waitFor(() => expect(daemon.bodies(REFRESH)).toEqual([{ keepTrelloCardIds: [] }]));
    await waitFor(() =>
      expect(M.S.toasts.map((t) => t.msg)).toContain("Read Trello. 2 new cards."),
    );
  });
});

describe("the marker on a card that is on Trello", () => {
  it("shows From Trello with its link and list, from the daemon's own card id", async () => {
    seed();
    show();
    await screen.findByRole("button", { name: /Apply to Trello/ });
    const card = M.card("api#41");
    if (!card) throw new Error("the store has no api#41");
    render(() => <CardMeta card={card} c={M.deco(card)} />);
    expect(screen.getByRole("link", { name: "From Trello" })).toHaveAttribute("href", URL_41);
    expect(screen.getByText("List: In review")).toBeInTheDocument();
    const other = M.card("api#42");
    if (!other) throw new Error("the store has no api#42");
    cleanup();
    render(() => <CardMeta card={other} c={M.deco(other)} />);
    expect(screen.queryByRole("link", { name: "From Trello" })).toBeNull();
  });
});
