import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { prototypeCards } from "~/testing/prototype-cards";
import { AgentsView } from "./AgentsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const seed = prototypeCards();
const DESKTOP_PX = 1440;
const TABLET_PX = 820;
const PHONE_PX = 390;

function showProject(pid: string, width = DESKTOP_PX): void {
  M.S.cards = structuredClone(seed);
  M.S.sort.agents = { k: "state", dir: 1 };
  M.S.focusId = null;
  M.S.openId = null;
  M.S.dialog = null;
  M.S.toasts = [];
  M.setViewport(width, 900);
  M.go("project", pid, "agents");
}

const dataCards = (): string[] =>
  screen
    .getAllByRole("row")
    .slice(1)
    .map((row) => row.getAttribute("data-card") ?? "");
const rowOf = (id: CardKey): HTMLElement => {
  const row = document.querySelector<HTMLElement>(`tr[data-card="${id}"]`);
  if (!row) throw new Error(`no row for #${id}`);
  return row;
};
const headerNames = (): string[] =>
  screen.getAllByRole("columnheader").map((h) => h.textContent ?? "");

beforeEach(() => {
  vi.useFakeTimers();
  showProject("api");
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe("AgentsView table", () => {
  it("shows the eleven columns at desktop width, sorted by state", () => {
    render(() => <AgentsView />);
    expect(headerNames()).toEqual([
      "Card",
      "Role",
      "Agent",
      "Model",
      "Thinking",
      "Permission mode",
      "State",
      "Session",
      "Current activity",
      "Cost",
      "Actions",
    ]);
    expect(screen.getByRole("columnheader", { name: "State" })).toHaveAttribute(
      "aria-sort",
      "ascending",
    );
    expect(screen.getByRole("columnheader", { name: "Role" })).toHaveAttribute("aria-sort", "none");
    expect(screen.getByRole("columnheader", { name: "Actions" })).toHaveAttribute(
      "aria-sort",
      "none",
    );
    expect(screen.getByRole("table")).toHaveStyle({ "min-width": "1180px" });
  });

  it("pins the Orchestrator first and leaves backlog cards out", () => {
    render(() => <AgentsView />);
    const orchestrator = screen.getByRole("row", { name: "Orchestrator session" });
    const rest = screen.getAllByRole("row").slice(2);
    expect(screen.getAllByRole("row")[1]).toBe(orchestrator);
    expect(orchestrator).toHaveAttribute("data-card", "");
    expect(orchestrator).toHaveTextContent("Orchestrator for api-gateway");
    expect(orchestrator).toHaveTextContent("Watching the board");
    expect(orchestrator).toHaveTextContent("$0.42");
    expect(orchestrator).toHaveTextContent("Plan only");
    expect(within(orchestrator).getByRole("button", { name: "Open project chat" })).toBeVisible();
    expect(rest.length).toBeGreaterThan(5);
    expect(dataCards()).not.toContain("api#45");
    expect(dataCards().slice(0, 5)).toEqual(["", "api#42", "api#41", "api#44", "api#43"]);
  });

  it("describes a card row: number, state, session, activity, and cost", () => {
    render(() => <AgentsView />);
    const row = rowOf("api#41");
    expect(row).toHaveAttribute("aria-label", "#41 Fix token refresh on login. Working");
    expect(row).toHaveTextContent("#41");
    expect(row).toHaveTextContent("Fix token refresh on login");
    expect(row).toHaveTextContent("Working");
    expect(row).toHaveTextContent("Awake");
    expect(row).toHaveTextContent("Running auth tests");
    expect(row).toHaveTextContent("$0.84");
    expect(row).toHaveTextContent("High");
  });

  it("shows the reason for a card that needs you, in the needs-you color", () => {
    render(() => <AgentsView />);
    expect(within(rowOf("api#43")).getByText("Plan ready for review")).toHaveClass(
      "text-status-needs-you-text",
    );
  });

  it("says Not supported for a model without thinking and Merged for a done card", () => {
    M.S.cards = structuredClone(seed);
    M.go("project", "api", "agents");
    const backlog = M.card("api#45");
    if (backlog) backlog.state = "working";
    render(() => <AgentsView />);
    expect(rowOf("api#45")).toHaveTextContent("Not supported");
    expect(rowOf("api#33")).toHaveTextContent("Merged");
    expect(rowOf("api#33")).toHaveTextContent("Stopped");
  });

  it("marks the focused card's row with the selected fill", () => {
    render(() => <AgentsView />);
    expect(rowOf("api#41")).not.toHaveClass("bg-surface-selected");
    M.set({ focusId: "api#41" });
    expect(rowOf("api#41")).toHaveClass("bg-surface-selected");
    M.set({ focusId: "api#43" });
    expect(rowOf("api#41")).not.toHaveClass("bg-surface-selected");
    expect(rowOf("api#43")).toHaveClass("bg-surface-selected");
  });

  it("opens the card on click and on Enter, and the chat from the Orchestrator row", () => {
    render(() => <AgentsView />);
    fireEvent.click(rowOf("api#43"));
    expect(M.S.openId).toBe("api#43");
    M.set({ openId: null });
    fireEvent.keyDown(rowOf("api#44"), { key: "Enter" });
    expect(M.S.openId).toBe("api#44");
    fireEvent.keyDown(rowOf("api#41"), { key: "a" });
    expect(M.S.openId).toBe("api#44");
    fireEvent.click(screen.getByRole("row", { name: "Orchestrator session" }));
    expect(M.S.route.view).toBe("chat");
  });

  it("does not open the card when Enter is pressed inside an action button", () => {
    render(() => <AgentsView />);
    fireEvent.keyDown(within(rowOf("api#41")).getByRole("button", { name: "Pin #41" }), {
      key: "Enter",
    });
    expect(M.S.openId).toBeNull();
  });
});

describe("AgentsView sorting", () => {
  it("sorts by a header, flips on a second click, and updates aria-sort", () => {
    render(() => <AgentsView />);
    fireEvent.click(screen.getByRole("button", { name: "Cost" }));
    expect(M.S.sort.agents).toEqual({ k: "cost", dir: 1 });
    expect(screen.getByRole("columnheader", { name: "Cost" })).toHaveAttribute(
      "aria-sort",
      "ascending",
    );
    expect(screen.getByRole("columnheader", { name: "State" })).toHaveAttribute(
      "aria-sort",
      "none",
    );
    const costs = screen
      .getAllByRole("row")
      .slice(2)
      .map((row) => Number(row.querySelector("td:nth-last-child(2)")?.textContent?.slice(1)));
    expect(costs).toEqual([...costs].sort((a, b) => a - b));
    fireEvent.click(screen.getByRole("button", { name: "Cost" }));
    expect(M.S.sort.agents).toEqual({ k: "cost", dir: -1 });
    expect(screen.getByRole("columnheader", { name: "Cost" })).toHaveAttribute(
      "aria-sort",
      "descending",
    );
  });

  it("keeps the same row element, and its focus, when the card changes", () => {
    render(() => <AgentsView />);
    const before = rowOf("api#41");
    before.focus();
    const card = M.card("api#41");
    if (card) card.doing = "Reading the diff";
    expect(rowOf("api#41")).toBe(before);
    expect(before).toHaveTextContent("Reading the diff");
    expect(document.activeElement).toBe(before);
    fireEvent.click(screen.getByRole("button", { name: "Card" }));
    expect(rowOf("api#41")).toBe(before);
  });
});

describe("AgentsView row actions", () => {
  it("runs an action without opening the card", () => {
    render(() => <AgentsView />);
    fireEvent.click(within(rowOf("api#43")).getByRole("button", { name: "Pin #43" }));
    expect(M.card("api#43")?.pinned).toBe(true);
    expect(M.S.openId).toBeNull();
    const unpin = within(rowOf("api#43")).getByRole("button", { name: "Unpin #43" });
    expect(unpin).toHaveAttribute("title", "Unpin");
    expect(within(rowOf("api#43")).getByText("Pinned")).toBeVisible();
  });

  it("keeps the button element when its label changes", () => {
    render(() => <AgentsView />);
    const pin = within(rowOf("api#43")).getByRole("button", { name: "Pin #43" });
    fireEvent.click(pin);
    expect(within(rowOf("api#43")).getByRole("button", { name: "Unpin #43" })).toBe(pin);
  });

  it("puts a card to sleep with the button and shows Wake", () => {
    render(() => <AgentsView />);
    fireEvent.click(within(rowOf("api#39")).getByRole("button", { name: "Sleep #39" }));
    expect(M.card("api#39")?.asleep).toBe(true);
    expect(within(rowOf("api#39")).getByRole("button", { name: "Wake #39" })).toBeVisible();
    expect(within(rowOf("api#39")).queryByRole("button", { name: /Stop session/ })).toBeNull();
    expect(rowOf("api#39")).toHaveTextContent("Asleep");
  });

  it("shows Waking while a sleeping card wakes, then Awake", () => {
    showProject("web");
    render(() => <AgentsView />);
    expect(rowOf("web#115")).toHaveTextContent("Asleep");
    fireEvent.click(within(rowOf("web#115")).getByRole("button", { name: "Wake #115" }));
    expect(rowOf("web#115")).toHaveTextContent("Waking");
    vi.advanceTimersByTime(2000);
    expect(rowOf("web#115")).toHaveTextContent("Awake");
  });

  it("asks before stopping a session, then shows the card asleep", () => {
    render(() => <AgentsView />);
    fireEvent.click(within(rowOf("api#41")).getByRole("button", { name: "Stop session on #41" }));
    expect(M.S.dialog?.title).toBe("Stop session");
    expect(M.S.openId).toBeNull();
    M.S.dialog?.run();
    expect(rowOf("api#41")).toHaveTextContent("Asleep");
    expect(rowOf("api#41")).not.toHaveTextContent("Running auth tests");
  });

  it("gives a done card only the Open button", () => {
    render(() => <AgentsView />);
    expect(
      within(rowOf("api#33"))
        .getAllByRole("button")
        .map((b) => b.getAttribute("aria-label")),
    ).toEqual(["Open #33"]);
  });
});

describe("AgentsView bypass card", () => {
  it("shows the Bypass badge and a bold red permission mode", () => {
    showProject("mobile");
    render(() => <AgentsView />);
    const row = rowOf("mobile#209");
    expect(within(row).getByText("Bypass")).toHaveClass("bg-bypass-bg");
    expect(within(row).getByText("Bypass permissions")).toHaveClass(
      "text-status-danger-text",
      "font-semibold",
    );
    expect(row).toHaveAttribute("aria-label", expect.stringContaining("Bypass permissions on"));
  });

  it("shows Pinned next to the session of a pinned card", () => {
    showProject("mobile");
    render(() => <AgentsView />);
    expect(within(rowOf("mobile#207")).getByText("Pinned")).toBeVisible();
  });
});

describe("AgentsView sizes", () => {
  it("hides Thinking, Permission mode, and Session below 1200 px", () => {
    showProject("api", TABLET_PX);
    render(() => <AgentsView />);
    expect(headerNames()).toEqual([
      "Card",
      "Role",
      "Agent",
      "Model",
      "State",
      "Current activity",
      "Cost",
      "Actions",
    ]);
    expect(screen.getByRole("table")).toHaveStyle({ "min-width": "820px" });
    expect(rowOf("api#41")).not.toHaveTextContent("Awake");
  });

  it("switches between table and phone list when the width changes", () => {
    render(() => <AgentsView />);
    expect(screen.getByRole("table")).toBeVisible();
    M.setViewport(PHONE_PX, 844);
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.getAllByRole("listitem").length).toBeGreaterThan(5);
    M.setViewport(DESKTOP_PX, 900);
    expect(screen.getByRole("table")).toBeVisible();
  });
});

describe("AgentsView phone list", () => {
  beforeEach(() => showProject("mobile", PHONE_PX));

  it("stacks the facts of each session under its title", () => {
    render(() => <AgentsView />);
    const [orchestrator, ...rest] = screen.getAllByRole("listitem");
    expect(orchestrator).toHaveTextContent("Orchestrator for mobile-app");
    expect(orchestrator).toHaveTextContent("AwakeAwakeOrchestratorClaude Codeclaude-opus-4-1$0.42");
    expect(orchestrator).toHaveTextContent("Plan onlyHigh");
    const bypass = rest.find((li) => li.textContent?.includes("#209"));
    expect(bypass).toHaveTextContent("Bypass permissions");
    expect(bypass).toHaveTextContent("Running ./gradlew");
    expect(within(bypass as HTMLElement).getByText("Bypass permissions").parentElement).toHaveClass(
      "text-status-danger-text",
    );
  });

  it("has 44 px action buttons that do not open the card", () => {
    render(() => <AgentsView />);
    const pin = screen.getByRole("button", { name: "Pin #209" });
    expect(pin).toHaveAttribute("data-compact", "1");
    expect(pin).toHaveClass("min-h-11");
    fireEvent.click(pin);
    expect(M.card("mobile#209")?.pinned).toBe(true);
    expect(M.S.openId).toBeNull();
    expect(screen.getByRole("button", { name: "Unpin #209" })).toBeVisible();
  });

  it("opens the card from the row button and the chat from the Orchestrator", () => {
    render(() => <AgentsView />);
    const row = screen.getAllByRole("listitem").find((li) => li.textContent?.includes("#209"));
    fireEvent.click(within(row as HTMLElement).getAllByRole("button")[0] as HTMLElement);
    expect(M.S.openId).toBe("mobile#209");
    fireEvent.click(screen.getByRole("button", { name: "Open project chat" }));
    expect(M.S.route.view).toBe("chat");
  });
});

describe("AgentsView empty project", () => {
  it("says there are no sessions but still shows the Orchestrator", () => {
    M.S.cards = structuredClone(seed).filter((c: { p: string }) => c.p !== "web");
    M.go("project", "web", "agents");
    render(() => <AgentsView />);
    expect(
      screen.getByText("No sessions yet. Start a card to give it an agent session."),
    ).toBeVisible();
    expect(dataCards()).toEqual([""]);
    expect(M.nav).toEqual({ owner: "agents", rows: [] });
  });

  it("does not show the message while there are sessions", () => {
    render(() => <AgentsView />);
    expect(screen.queryByText(/No sessions yet/)).toBeNull();
  });
});

describe("AgentsView keyboard navigation", () => {
  it("publishes the card order to the shell, and clears it on unmount", () => {
    const { unmount } = render(() => <AgentsView />);
    expect(M.nav?.owner).toBe("agents");
    expect(M.nav?.rows?.slice(0, 4)).toEqual(["api#42", "api#41", "api#44", "api#43"]);
    fireEvent.click(screen.getByRole("button", { name: "Card" }));
    expect(M.nav?.rows?.[0]).toBe("api#33");
    unmount();
    expect(M.nav).toBeNull();
  });

  it("leaves another view's navigation alone on unmount", () => {
    const { unmount } = render(() => <AgentsView />);
    M.nav = { owner: "list", rows: ["api#1"] };
    unmount();
    expect(M.nav).toEqual({ owner: "list", rows: ["api#1"] });
    M.nav = null;
  });
});
