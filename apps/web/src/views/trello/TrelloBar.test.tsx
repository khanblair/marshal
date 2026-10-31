import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { mockTrello } from "~/mock/trello";
import { BoardView } from "../board/BoardView";
import { reset } from "../board/board-test-utils";
import { TrelloBar } from "./TrelloBar";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const PHONE_PX = 390;
const HEIGHT_PX = 900;

const put = (view = mockTrello()) =>
  M.set({ trello: { web: { pending: view, error: "", loading: false } } });

beforeEach(() => {
  reset("web");
  M.set({ trello: {}, trelloApply: null });
});
afterEach(() => {
  cleanup();
  M.set({ trello: {}, trelloApply: null });
});

describe("the Trello strip of a board", () => {
  it("draws nothing for a project with no Trello board, or one that was never read", () => {
    render(() => <TrelloBar pid="web" />);
    expect(document.querySelector("[data-trello-bar]")).toBeNull();
    put({ ...mockTrello(), linked: false, changes: [], cards: [] });
    expect(document.querySelector("[data-trello-bar]")).toBeNull();
    expect(screen.queryByRole("button", { name: /Trello/ })).toBeNull();
  });

  it("names the board and how many changes wait, on the Apply to Trello button", () => {
    put();
    render(() => <TrelloBar pid="web" />);
    expect(screen.getByText("Web dashboard")).toBeInTheDocument();
    expect(screen.getByText("Last read 4 min ago")).toBeInTheDocument();
    const apply = screen.getByRole("button", { name: /Apply to Trello/ });
    expect(apply).toHaveTextContent("5");
    expect(screen.getByRole("button", { name: "Refresh from Trello" })).toBeInTheDocument();
  });

  it("shows Apply to Trello with no count when nothing waits", () => {
    put({ ...mockTrello(), changes: [] });
    render(() => <TrelloBar pid="web" />);
    expect(screen.getByRole("button", { name: "Apply to Trello" })).toBeInTheDocument();
  });

  it("opens the dialog for the project, and writes nothing", () => {
    put();
    render(() => <TrelloBar pid="web" />);
    fireEvent.click(screen.getByRole("button", { name: /Apply to Trello/ }));
    expect(M.S.trelloApply).toEqual({ pid: "web" });
    expect(M.S.trello?.web?.pending?.changes).toHaveLength(5);
  });

  it("reads the board again on Refresh from Trello, and says what it found", async () => {
    put();
    render(() => <TrelloBar pid="web" />);
    fireEvent.click(screen.getByRole("button", { name: "Refresh from Trello" }));
    await waitFor(() =>
      expect(M.S.toasts.map((t) => t.msg)).toContain("Read Trello. Nothing is new."),
    );
    expect(M.S.trello?.web?.pending?.changes).toHaveLength(5);
  });

  it("keeps both actions on a phone, the refresh as an icon button with its name", () => {
    M.setViewport(PHONE_PX, HEIGHT_PX);
    put();
    render(() => <TrelloBar pid="web" />);
    const refresh = screen.getByRole("button", { name: "Refresh from Trello" });
    expect(refresh).toHaveAttribute("title", "Refresh from Trello");
    expect(refresh).not.toHaveTextContent("Refresh");
    expect(screen.getByRole("button", { name: /Apply to Trello/ })).toBeInTheDocument();
  });

  it("sits above the columns of a linked project's board", () => {
    put();
    render(() => <BoardView />);
    const bar = document.querySelector("[data-trello-bar]");
    const columns = document.querySelector("section[data-col]");
    expect(bar).not.toBeNull();
    expect(bar?.compareDocumentPosition(columns as Node)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
  });
});
