import { fireEvent, render, screen, within } from "@solidjs/testing-library";
import { describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { GOLDEN_CATALOG } from "~/testing/agents";
import { useCatalog } from "~/testing/test-store";
import { BoardView } from "./BoardView";
import { cardIds, column, useBoardTestStore } from "./board-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

useBoardTestStore();

describe("BoardView quick add", () => {
  const openForm = (col: string): HTMLTextAreaElement => {
    fireEvent.click(within(column(col)).getByRole("button", { name: "Add a card" }));
    vi.runAllTimers();
    return within(column(col)).getByRole("textbox", { name: "Card title" });
  };

  it("offers Add a card only in Backlog, Planning, and Working", () => {
    render(() => <BoardView />);
    expect(screen.getAllByRole("button", { name: "Add a card" })).toHaveLength(3);
    expect(
      within(column("needs")).queryByRole("button", { name: "Add a card" }),
    ).not.toBeInTheDocument();
  });

  it("opens a focused form with the column's hint", () => {
    render(() => <BoardView />);
    const field = openForm("backlog");
    expect(M.S.quickAddAt).toBe("all:backlog");
    expect(field).toHaveAttribute("placeholder", "Enter a title for this card");
    expect(field).toHaveFocus();
    expect(
      within(column("backlog")).getByText("The card waits in Backlog until you start it."),
    ).toBeInTheDocument();
    expect(within(column("backlog")).getByRole("button", { name: "Add card" })).toHaveAttribute(
      "type",
      "submit",
    );
    expect(
      within(column("backlog")).queryByRole("button", { name: "Add a card" }),
    ).not.toBeInTheDocument();
  });

  it("says a Working card starts right away", () => {
    render(() => <BoardView />);
    openForm("working");
    expect(
      within(column("working")).getByText("The agent starts working on this card right away."),
    ).toBeInTheDocument();
    expect(
      within(column("working")).getByRole("button", { name: "Add and start" }),
    ).toBeInTheDocument();
  });

  it("adds a card on Enter, clears the field, and keeps the form open", () => {
    render(() => <BoardView />);
    const field = openForm("backlog");
    const before = M.S.cards.length;
    field.value = "  A brand new card  ";
    fireEvent.keyDown(field, { key: "Enter" });
    expect(M.S.cards).toHaveLength(before + 1);
    const created = M.S.cards[M.S.cards.length - 1];
    expect(created).toMatchObject({ title: "A brand new card", state: "backlog", p: "api" });
    expect(field.value).toBe("");
    expect(M.S.quickAddAt).toBe("all:backlog");
    expect(cardIds(column("backlog"))).toContain(created?.id);
  });

  it("does not add on Shift and Enter or an empty title", () => {
    render(() => <BoardView />);
    const field = openForm("backlog");
    const before = M.S.cards.length;
    field.value = "Line one";
    fireEvent.keyDown(field, { key: "Enter", shiftKey: true });
    field.value = "   ";
    fireEvent.keyDown(field, { key: "Enter" });
    expect(M.S.cards).toHaveLength(before);
  });

  it("adds and starts a card from the Working form with the submit button", () => {
    render(() => <BoardView />);
    const field = openForm("working");
    field.value = "Start me now";
    fireEvent.click(within(column("working")).getByRole("button", { name: "Add and start" }));
    const created = M.S.cards[M.S.cards.length - 1];
    expect(created?.title).toBe("Start me now");
    expect(created?.state).toBe("working");
    expect(field.value).toBe("");
  });

  it("closes on Escape without letting the shell's window handler see the key", () => {
    render(() => <BoardView />);
    const field = openForm("backlog");
    const windowKey = vi.fn();
    window.addEventListener("keydown", windowKey);
    fireEvent.keyDown(field, { key: "Escape" });
    window.removeEventListener("keydown", windowKey);
    expect(windowKey).not.toHaveBeenCalled();
    expect(M.S.quickAddAt).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Card title" })).not.toBeInTheDocument();
  });

  it("closes with the Cancel button", () => {
    render(() => <BoardView />);
    openForm("planning");
    fireEvent.click(within(column("planning")).getByRole("button", { name: "Cancel" }));
    expect(M.S.quickAddAt).toBeNull();
  });

  it("keeps the same field while the store changes", () => {
    render(() => <BoardView />);
    const field = openForm("backlog");
    field.value = "half typed";
    const card = M.S.cards.find((c) => c.state === "working");
    if (card) card.upd += 1;
    M.S.cards[0]?.labels.push("extra");
    M.S.query.api = "";
    expect(within(column("backlog")).getByRole("textbox", { name: "Card title" })).toBe(field);
    expect(field.value).toBe("half typed");
  });

  it("opens the New card dialog from a template, starting the card outside Backlog", () => {
    render(() => <BoardView />);
    fireEvent.click(
      within(column("backlog")).getByRole("button", { name: "Create from a template" }),
    );
    expect(M.S.newCard).toMatchObject({ template: "Bug fix", start: false });
    fireEvent.click(
      within(column("working")).getByRole("button", { name: "Create from a template" }),
    );
    expect(M.S.newCard).toMatchObject({ template: "Bug fix", start: true });
    openForm("planning");
    fireEvent.click(
      within(column("planning")).getByRole("button", { name: "Create from a template" }),
    );
    expect(M.S.newCard).toMatchObject({ template: "Bug fix", start: true });
  });

  it("gives a card added in a role lane that role, and a template card too", () => {
    M.S.swim.api = "role";
    render(() => <BoardView />);
    const testerBacklog = document.querySelectorAll<HTMLElement>('section[data-col="backlog"]')[2];
    if (!testerBacklog) throw new Error("no Tester lane");
    fireEvent.click(within(testerBacklog).getByRole("button", { name: "Add a card" }));
    vi.runAllTimers();
    const field = within(testerBacklog).getByRole<HTMLTextAreaElement>("textbox", {
      name: "Card title",
    });
    field.value = "Test the lane";
    fireEvent.keyDown(field, { key: "Enter" });
    expect(M.S.cards[M.S.cards.length - 1]).toMatchObject({
      title: "Test the lane",
      role: "Tester",
    });
    fireEvent.click(within(testerBacklog).getByRole("button", { name: "Create from a template" }));
    expect(M.S.newCard).toMatchObject({ role: "Tester" });
  });

  it("sets the agent and its first model for a card added in an agent lane", () => {
    M.S.swim.api = "agent";
    render(() => <BoardView />);
    const codex = screen.getByRole("button", { name: /^Codex/ });
    const lane = codex.nextElementSibling;
    const backlog = lane?.querySelector<HTMLElement>('section[data-col="backlog"]');
    if (!backlog) throw new Error("no Codex lane");
    fireEvent.click(within(backlog).getByRole("button", { name: "Add a card" }));
    vi.runAllTimers();
    const field = within(backlog).getByRole<HTMLTextAreaElement>("textbox", { name: "Card title" });
    field.value = "Codex card";
    fireEvent.keyDown(field, { key: "Enter" });
    expect(M.S.cards[M.S.cards.length - 1]).toMatchObject({ agent: "Codex", model: "gpt-5-codex" });
  });

  describe("in the lane of an agent that is not installed", () => {
    const codexBacklog = (): HTMLElement => {
      const lane = screen.getByRole("button", { name: /^Codex/ }).nextElementSibling;
      const backlog = lane?.querySelector<HTMLElement>('section[data-col="backlog"]');
      if (!backlog) throw new Error("no Codex lane");
      return backlog;
    };

    it("gives the new card the default agent, not the agent that cannot be used", () => {
      const restore = useCatalog(M, GOLDEN_CATALOG);
      M.S.swim.api = "agent";
      render(() => <BoardView />);
      const backlog = codexBacklog();
      fireEvent.click(within(backlog).getByRole("button", { name: "Add a card" }));
      vi.runAllTimers();
      const field = within(backlog).getByRole<HTMLTextAreaElement>("textbox", {
        name: "Card title",
      });
      field.value = "Card for a missing agent";
      fireEvent.keyDown(field, { key: "Enter" });
      expect(M.S.cards[M.S.cards.length - 1]).toMatchObject({
        title: "Card for a missing agent",
        agent: "Claude Code",
      });
      restore();
    });

    it("opens the New card dialog on the default agent, not the missing one", () => {
      const restore = useCatalog(M, GOLDEN_CATALOG);
      M.S.swim.api = "agent";
      render(() => <BoardView />);
      const backlog = codexBacklog();
      fireEvent.click(within(backlog).getByRole("button", { name: "Create from a template" }));
      expect(M.S.newCard).toMatchObject({ template: "Bug fix", agent: "Claude Code" });
      restore();
    });
  });
});
