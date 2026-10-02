import { cleanup, fireEvent, screen, within } from "@solidjs/testing-library";
import { afterEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { INTEGRATIONS_EXPANDED_KEY, INTEGRATIONS_VIEW_KEY } from "./integrations-view";
import { lastToast, showSettings } from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

afterEach(() => {
  cleanup();
  window.localStorage.removeItem(INTEGRATIONS_VIEW_KEY);
  window.localStorage.removeItem(INTEGRATIONS_EXPANDED_KEY);
});

/** The GitHub row's own panel, so a button is found in that row rather than in another one. */
const githubPanel = (): HTMLElement => {
  const panel = screen.getByText("GitHub").closest("[data-integration]");
  if (!(panel instanceof HTMLElement)) throw new Error("the GitHub row is not drawn");
  return panel;
};
const githubRow = () => M.S.integrations.find((integration) => integration.id === "github");

// A store with no daemon keeps the design's own connections: pressing a row's button turns its
// state in the mock, and the GitHub dialog (the daemon's own sign-in) never opens.
describe("the Integrations section on the mock", () => {
  it("opens nothing for a connected GitHub, and says so", () => {
    showSettings("integrations");
    expect(M.connectionOnDaemon("github")).toBe(false);
    fireEvent.click(within(githubPanel()).getByRole("button", { name: "Manage" }));
    expect(lastToast()).toBe("GitHub settings opened");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("connects a GitHub that is not connected by flipping its row, with no dialog", () => {
    showSettings("integrations");
    M.set({
      integrations: M.S.integrations.map((entry) =>
        entry.id === "github" ? { ...entry, st: "none" as const, detail: "" } : entry,
      ),
    });
    fireEvent.click(within(githubPanel()).getByRole("button", { name: "Connect" }));
    expect(githubRow()?.st).toBe("connected");
    expect(lastToast()).toBe("GitHub connected");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});

describe("the Integrations layout toggle", () => {
  const layout = (): string | null =>
    document.querySelector("[data-view]")?.getAttribute("data-view") ?? null;
  const choose = (name: "List view" | "Card view") =>
    fireEvent.click(screen.getByRole("button", { name }));

  it("shows a list by default, with every connection in it", () => {
    showSettings("integrations");
    expect(layout()).toBe("list");
    expect(screen.getByRole("button", { name: "List view" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "Card view" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    for (const name of ["GitHub", "Trello", "Gmail"]) {
      expect(screen.getByText(name)).toBeInTheDocument();
    }
  });

  it("switches to cards and back, and a new screen reads the choice", () => {
    showSettings("integrations");
    choose("Card view");
    expect(layout()).toBe("cards");
    expect(screen.getByRole("button", { name: "Card view" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(window.localStorage.getItem(INTEGRATIONS_VIEW_KEY)).toBe("cards");
    cleanup();
    showSettings("integrations");
    expect(layout()).toBe("cards");
    choose("List view");
    expect(layout()).toBe("list");
    expect(window.localStorage.getItem(INTEGRATIONS_VIEW_KEY)).toBe("list");
  });

  it("keeps a connection's button working in both layouts", () => {
    showSettings("integrations");
    fireEvent.click(within(githubPanel()).getByRole("button", { name: "Manage" }));
    expect(lastToast()).toBe("GitHub settings opened");
    choose("Card view");
    fireEvent.click(within(githubPanel()).getByRole("button", { name: "Manage" }));
    expect(lastToast()).toBe("GitHub settings opened");
  });

  it("falls back to a list when the remembered choice is not one", () => {
    window.localStorage.setItem(INTEGRATIONS_VIEW_KEY, "table");
    showSettings("integrations");
    expect(layout()).toBe("list");
  });
});

describe("folding a connection", () => {
  const detail = "GitHub App installed on 3 repositories";
  const inGithub = () => within(githubPanel());
  const press = (name: string) => fireEvent.click(screen.getByRole("button", { name }));

  it("starts folded to the header and the buttons, and unfolds from the chevron, in both layouts", () => {
    showSettings("integrations");
    expect(inGithub().queryByText(detail)).not.toBeInTheDocument();
    expect(inGithub().getByRole("button", { name: "Manage" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Expand GitHub" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    press("Expand GitHub");
    expect(inGithub().getByText(detail)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Collapse GitHub" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    // The fold belongs to the connection, so the cards show it unfolded too, and can fold it again.
    press("Card view");
    expect(inGithub().getByText(detail)).toBeInTheDocument();
    press("Collapse GitHub");
    expect(inGithub().queryByText(detail)).not.toBeInTheDocument();
    expect(inGithub().getByRole("button", { name: "Manage" })).toBeInTheDocument();
  });

  it("remembers which connections are unfolded, and a new screen starts that way", () => {
    showSettings("integrations");
    press("Expand GitHub");
    expect(window.localStorage.getItem(INTEGRATIONS_EXPANDED_KEY)).toBe('["github"]');
    cleanup();
    showSettings("integrations");
    expect(screen.getByRole("button", { name: "Collapse GitHub" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Expand Trello" })).toBeInTheDocument();
    press("Collapse GitHub");
    expect(window.localStorage.getItem(INTEGRATIONS_EXPANDED_KEY)).toBe("[]");
  });

  it("ignores a remembered fold that is not a list of ids", () => {
    window.localStorage.setItem(INTEGRATIONS_EXPANDED_KEY, "{not json");
    showSettings("integrations");
    expect(screen.getByRole("button", { name: "Expand GitHub" })).toBeInTheDocument();
  });

  it("has no Test button on a connection the daemon does not own", () => {
    showSettings("integrations");
    expect(inGithub().queryByRole("button", { name: "Test" })).not.toBeInTheDocument();
  });
});
