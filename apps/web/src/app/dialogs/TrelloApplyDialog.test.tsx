import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TrelloView } from "~/data/mappers/trello";
import { M } from "~/mock";
import { mockTrello } from "~/mock/trello";
import { TrelloApplyDialog } from "./TrelloApplyDialog";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * The dialog over a store with no daemon: the list is what the store holds, and the apply, the skip,
 * and the keep work on it (`mock/trello.ts`). The fixture is a linked project with five changes on
 * three cards, one of them (a move to Done on web#118) in conflict. The daemon's own answers, a
 * change it refuses with a reason, are in `TrelloApplyDialog.daemon.test.tsx`.
 */
const DESKTOP_PX = 1440;
const PHONE_PX = 390;
const HEIGHT_PX = 900;

function show(view: TrelloView = mockTrello()) {
  M.set({ trello: { web: { pending: view, error: "", loading: false } } });
  M.openTrelloApply("web");
  return render(() => <TrelloApplyDialog />);
}

const dialog = () => screen.getByRole("dialog", { name: "Apply to Trello" });
const box = (name: RegExp | string) => screen.getByRole("checkbox", { name });
const press = (name: RegExp | string) => fireEvent.click(screen.getByRole("button", { name }));
const changeIds = () => M.S.trello?.web?.pending?.changes.map((c) => c.id) ?? [];

beforeEach(() => {
  M.setViewport(DESKTOP_PX, HEIGHT_PX);
  M.set({ trello: {}, trelloApply: null, toasts: [] });
});
afterEach(() => {
  cleanup();
  M.set({ trello: {}, trelloApply: null });
});

describe("the changes to pick from", () => {
  it("lists them by card, with the card's key, and says who made each inside Marshal", () => {
    show();
    const first = within(screen.getByRole("region", { name: "Changes to send to Trello" }));
    expect(first.getByRole("heading", { name: /Dark mode for settings page/ })).toBeInTheDocument();
    expect(first.getByText("web#118")).toBeInTheDocument();
    expect(first.getByRole("heading", { name: /Migrate tables/ })).toBeInTheDocument();
    expect(first.getAllByText("by you")).toHaveLength(2);
    expect(first.getAllByText("by the agent")).toHaveLength(2);
    // A change with no known author has no "by" line.
    const create = box(/Create this card in Trello/).closest("li");
    expect(create && within(create).queryByText(/^by /)).toBeNull();
  });

  it("ticks every change except the one in conflict, and counts the ticked ones", () => {
    show();
    expect(box(/Add a comment/)).toBeChecked();
    expect(box(/Tick Check contrast/)).toBeChecked();
    expect(box(/Change the description/)).toBeChecked();
    expect(box(/Move to Done/)).not.toBeChecked();
    expect(screen.getByRole("button", { name: "Apply 4 changes" })).toBeEnabled();
  });

  it("changes the count as boxes are ticked, and turns Apply off at none", () => {
    show();
    fireEvent.click(box(/Add a comment/));
    expect(screen.getByRole("button", { name: "Apply 3 changes" })).toBeInTheDocument();
    for (const name of [/Tick Check contrast/, /Change the description/, /Create this card/]) {
      fireEvent.click(box(name));
    }
    expect(screen.getByRole("button", { name: "Apply 0 changes" })).toBeDisabled();
    fireEvent.click(box(/Add a comment/));
    expect(screen.getByRole("button", { name: "Apply 1 change" })).toBeEnabled();
  });

  it("cuts a long comment to a few lines and opens it on request, and leaves a short one alone", () => {
    const view = mockTrello();
    const long = "The export now streams rows. ".repeat(12);
    view.changes = view.changes.map((c) =>
      c.id === "mock-comment-118" ? { ...c, detail: long } : c,
    );
    show(view);
    const row = box(/Add a comment/).closest("li") as HTMLElement;
    const text = within(row).getByText(/The export now streams rows/);
    expect(text).toHaveClass("line-clamp-3");
    const more = within(row).getByRole("button", { name: "Show more" });
    expect(more).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(more);
    expect(text).not.toHaveClass("line-clamp-3");
    expect(within(row).getByRole("button", { name: "Show less" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    const short = box(/Change the description/).closest("li") as HTMLElement;
    expect(within(short).queryByRole("button", { name: /Show/ })).toBeNull();
  });
});

describe("a change in conflict", () => {
  it("says what Trello changed and offers three ways out", () => {
    show();
    const row = box(/Move to Done/).closest("li") as HTMLElement;
    expect(within(row).getByText("Trello moved this card to Review.")).toBeInTheDocument();
    for (const name of ["Apply anyway", "Skip", "Keep Trello's version"]) {
      expect(within(row).getByRole("button", { name })).toBeEnabled();
    }
  });

  it("ticks the box together with Apply anyway, and clears both on the second press", () => {
    show();
    const row = box(/Move to Done/).closest("li") as HTMLElement;
    const anyway = within(row).getByRole("button", { name: "Apply anyway" });
    expect(anyway).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(anyway);
    expect(box(/Move to Done/)).toBeChecked();
    expect(anyway).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Apply 5 changes" })).toBeInTheDocument();
    fireEvent.click(anyway);
    expect(box(/Move to Done/)).not.toBeChecked();
    expect(anyway).toHaveAttribute("aria-pressed", "false");
  });

  it("treats ticking the box as Apply anyway, so a ticked conflict is never skipped silently", () => {
    show();
    const row = box(/Move to Done/).closest("li") as HTMLElement;
    fireEvent.click(box(/Move to Done/));
    expect(within(row).getByRole("button", { name: "Apply anyway" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  it("hides the change on Skip and leaves the others as they were", async () => {
    show();
    const row = box(/Move to Done/).closest("li") as HTMLElement;
    fireEvent.click(within(row).getByRole("button", { name: "Skip" }));
    await waitFor(() => expect(changeIds()).not.toContain("mock-move-118"));
    expect(screen.queryByRole("checkbox", { name: /Move to Done/ })).toBeNull();
    expect(await screen.findByRole("button", { name: "Apply 4 changes" })).toBeInTheDocument();
  });

  it("sets the card's title, description, and ticks back to Trello's, and says the rest is still pending", async () => {
    show();
    const row = box(/Move to Done/).closest("li") as HTMLElement;
    fireEvent.click(within(row).getByRole("button", { name: "Keep Trello's version" }));
    // The tick on the same card is dropped; the move is not one of the kinds that is set back.
    await waitFor(() => expect(changeIds()).not.toContain("mock-tick-118"));
    expect(changeIds()).toContain("mock-move-118");
    expect(changeIds()).toContain("mock-comment-118");
    expect(
      await screen.findByText(/Kept Trello's version of this card's title/),
    ).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Apply 3 changes" })).toBeInTheDocument();
  });
});

describe("applying", () => {
  it("writes the ticked changes, leaves the conflict pending, and says what was written", async () => {
    show();
    press("Apply 4 changes");
    const written = await screen.findByRole("region", { name: "Written to Trello" });
    expect(within(written).getByRole("heading")).toHaveTextContent("Written to Trello (4)");
    expect(within(written).getByText("Add a comment")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Not written" })).toBeNull();
    expect(changeIds()).toEqual(["mock-move-118"]);
    expect(screen.getByRole("heading", { name: "Sent to Trello" })).toBeInTheDocument();
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("writes a conflict too once it is applied anyway", async () => {
    show();
    fireEvent.click(screen.getByRole("button", { name: "Apply anyway" }));
    press("Apply 5 changes");
    const written = await screen.findByRole("region", { name: "Written to Trello" });
    expect(within(written).getByRole("heading")).toHaveTextContent("Written to Trello (5)");
    expect(within(written).getByText("Move to Done")).toBeInTheDocument();
    expect(changeIds()).toEqual([]);
  });

  it("moves focus to the report, and Done closes the dialog", async () => {
    show();
    press("Apply 4 changes");
    const heading = await screen.findByRole("heading", { name: "Sent to Trello" });
    await waitFor(() => expect(heading).toHaveFocus());
    press("Done");
    expect(M.S.trelloApply).toBeNull();
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("with nothing to send", () => {
  it("says so, and offers only Close", () => {
    show({ ...mockTrello(), changes: [] });
    expect(screen.getByText("Nothing to send to Trello.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Apply/ })).toBeNull();
    expect(screen.queryByRole("checkbox")).toBeNull();
    const footer = document.querySelector<HTMLElement>("[data-trello-footer]") as HTMLElement;
    expect(within(footer).getAllByRole("button")).toHaveLength(1);
    fireEvent.click(within(footer).getByRole("button", { name: "Close" }));
    expect(M.S.trelloApply).toBeNull();
  });

  it("names the board and when Marshal last read it", () => {
    show();
    expect(within(dialog()).getByText("Board: Web dashboard")).toBeInTheDocument();
    expect(within(dialog()).getByText("Last read 4 min ago")).toBeInTheDocument();
  });
});

describe("leaving and the keyboard", () => {
  it("closes with Cancel, the close button, and Escape, and writes nothing", () => {
    show();
    press("Cancel");
    expect(M.S.trelloApply).toBeNull();
    expect(changeIds()).toHaveLength(5);
    cleanup();
    show();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(M.S.trelloApply).toBeNull();
    cleanup();
    show();
    fireEvent.keyDown(dialog(), { key: "Escape" });
    expect(M.S.trelloApply).toBeNull();
    expect(changeIds()).toHaveLength(5);
  });

  it("puts focus on the list when it opens, so the keyboard can pick and scroll", async () => {
    show();
    const list = screen.getByRole("region", { name: "Changes to send to Trello" });
    await waitFor(() => expect(list).toHaveFocus());
    expect(list).toHaveAttribute("tabindex", "0");
  });

  it("is a bottom sheet on a phone", () => {
    M.setViewport(PHONE_PX, HEIGHT_PX);
    show();
    expect(dialog()).toHaveClass("bottom-0", "rounded-t-xl");
  });

  it("draws nothing while no project's dialog is open", () => {
    render(() => <TrelloApplyDialog />);
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
