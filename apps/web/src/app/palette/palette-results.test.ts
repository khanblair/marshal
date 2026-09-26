import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import type { PaletteHits } from "~/sync/search";
import { paletteResults, scrollOptionIntoView, stepSelection } from "./palette-results";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => {
  M.go("project", "api", "board");
  M.set({ openId: null });
});

const groups = (list: { group: string }[]): string[] => [...new Set(list.map((x) => x.group))];

describe("paletteResults", () => {
  it("with no query lists actions, projects, and settings, then the cards that need you", () => {
    const list = paletteResults(M, "");
    expect(groups(list)).toEqual(["Actions", "Projects", "Settings", "Cards that need you"]);
    const needs = list.filter((x) => x.group === "Cards that need you");
    expect(needs.map((x) => x.card)).toEqual(["api#43", "web#119", "mobile#210", "api#44"]);
    // A card row names its project, so cards of two projects with the same number differ.
    expect(needs[0]?.label).toBe("api-gateway #43 Add rate limiting per API key");
    expect(needs[1]?.label.startsWith("web-dashboard #119 ")).toBe(true);
    expect(list.some((x) => x.group === "Cards")).toBe(false);
  });

  it("treats a blank query as no query", () => {
    expect(paletteResults(M, "   ").map((x) => x.label)).toEqual(
      paletteResults(M, "").map((x) => x.label),
    );
  });

  it("keeps commands that contain every word, in any order, ignoring case", () => {
    const labels = paletteResults(M, "LIMITING rate").map((x) => x.label);
    expect(labels).toEqual(["api-gateway #43 Add rate limiting per API key"]);
  });

  it("also searches the hint and the group name", () => {
    expect(paletteResults(M, "go").some((x) => x.group === "Projects")).toBe(true);
    const settings = paletteResults(M, "settings theme");
    expect(settings.map((x) => x.label)).toContain("Theme");
    const cards = paletteResults(M, "api-gateway");
    expect(cards.some((x) => x.group === "Cards")).toBe(true);
  });

  it("returns nothing for a query that matches nothing", () => {
    expect(paletteResults(M, "zzzz nothing")).toEqual([]);
  });

  it("shows at most 60 rows", () => {
    const many = Array.from({ length: 80 }, (_, i) => ({
      group: "Actions",
      label: `Command ${i}`,
      icon: "plus",
      run: () => {},
    }));
    vi.spyOn(M, "commands").mockReturnValue(many);
    expect(paletteResults(M, "command")).toHaveLength(60);
    expect(paletteResults(M, "")).toHaveLength(60);
    vi.restoreAllMocks();
  });
});

/** A row the daemon's answer made: the palette's own command shape, with nothing it can run. */
const row = (group: string, label: string, over: Record<string, unknown> = {}) => ({
  group,
  label,
  icon: "plus",
  run: () => {},
  ...over,
});

const answer = (query: string, over: Partial<PaletteHits> = {}): PaletteHits => ({
  query,
  projects: [row("Projects", "api-gateway", { hint: "Go" })],
  cards: [row("Cards", "api-gateway #41 Fix token refresh on login", { card: "api#41" })],
  chats: [row("Chats", "Token rotation question", { hint: "api-gateway" })],
  ...over,
});

describe("paletteResults with the daemon's answer", () => {
  it("shows the matching actions, then the daemon's projects, cards, and chats, then the settings", () => {
    const list = paletteResults(M, "a", answer("a"));
    expect(groups(list)).toEqual(["Actions", "Projects", "Cards", "Chats", "Settings"]);
  });

  it("puts the daemon's rows in place of the store's own project and card rows", () => {
    const list = paletteResults(M, "api", answer("api"));
    expect(list.filter((x) => x.group === "Projects").map((x) => x.label)).toEqual(["api-gateway"]);
    expect(list.filter((x) => x.group === "Cards").map((x) => x.label)).toEqual([
      "api-gateway #41 Fix token refresh on login",
    ]);
    // The store has more api cards than the one row: those are the store's, and they are not shown.
    expect(list.filter((x) => x.group === "Cards")).toHaveLength(1);
  });

  it("does not filter the daemon's rows again, since it matched them by fields the row does not show", () => {
    const list = paletteResults(M, "api#41", answer("api#41"));
    expect(list.map((x) => x.label)).toContain("api-gateway #41 Fix token refresh on login");
  });

  it("keeps the order the daemon gave, best match first, inside each kind", () => {
    const hits = answer("x", {
      cards: [row("Cards", "second best"), row("Cards", "best"), row("Cards", "worst")],
    });
    const cards = paletteResults(M, "x", hits).filter((x) => x.group === "Cards");
    expect(cards.map((x) => x.label)).toEqual(["second best", "best", "worst"]);
  });

  it("ignores an answer to another query, and searches the store as it would with no answer", () => {
    const stale = answer("ap");
    expect(paletteResults(M, "api", stale).map((x) => x.label)).toEqual(
      paletteResults(M, "api").map((x) => x.label),
    );
    expect(paletteResults(M, "api", stale).some((x) => x.group === "Chats")).toBe(false);
  });

  it("reads the answer's query the way the daemon writes it: trimmed, with the spaces collapsed", () => {
    const hits = answer("rate limiting");
    expect(paletteResults(M, "  rate    limiting ", hits).some((x) => x.group === "Chats")).toBe(
      true,
    );
    expect(paletteResults(M, "Rate limiting", hits).some((x) => x.group === "Chats")).toBe(false);
  });

  it("uses no answer for a blank query, which lists the palette's own commands", () => {
    const blank = paletteResults(M, "  ", answer(""));
    expect(groups(blank)).toEqual(["Actions", "Projects", "Settings", "Cards that need you"]);
  });

  it("still shows at most 60 rows", () => {
    const many = (group: string) =>
      Array.from({ length: 40 }, (_, i) => row(group, `${group} ${i}`));
    const hits = answer("x", {
      projects: many("Projects"),
      cards: many("Cards"),
      chats: many("Chats"),
    });
    expect(paletteResults(M, "x", hits).length).toBeLessThanOrEqual(60);
  });
});

describe("stepSelection", () => {
  it("moves one row and stops at both ends", () => {
    expect(stepSelection(0, "ArrowDown", 3)).toBe(1);
    expect(stepSelection(2, "ArrowDown", 3)).toBe(2);
    expect(stepSelection(0, "ArrowUp", 3)).toBe(0);
    expect(stepSelection(2, "ArrowUp", 3)).toBe(1);
    expect(stepSelection(0, "ArrowDown", 0)).toBe(0);
  });
});

describe("scrollOptionIntoView", () => {
  function listWith(top: number, height: number, scrollTop: number, clientHeight: number) {
    const list = document.createElement("div");
    const parent = document.createElement("div");
    const option = document.createElement("button");
    option.dataset.pi = "4";
    Object.defineProperty(option, "offsetTop", { value: top });
    Object.defineProperty(option, "offsetHeight", { value: height });
    Object.defineProperty(parent, "clientHeight", { value: clientHeight });
    parent.scrollTop = scrollTop;
    parent.append(option);
    list.append(parent);
    return { list, parent };
  }

  it("scrolls up to show an option above the view, with an 8 px margin", () => {
    const { list, parent } = listWith(100, 36, 300, 200);
    scrollOptionIntoView(list, 4);
    expect(parent.scrollTop).toBe(92);
  });

  it("scrolls down to show an option below the view, with an 8 px margin", () => {
    const { list, parent } = listWith(400, 36, 100, 200);
    scrollOptionIntoView(list, 4);
    expect(parent.scrollTop).toBe(400 + 36 - 200 + 8);
  });

  it("leaves the scroll alone when the option is visible or missing", () => {
    const { list, parent } = listWith(150, 36, 100, 200);
    scrollOptionIntoView(list, 4);
    expect(parent.scrollTop).toBe(100);
    scrollOptionIntoView(list, 9);
    expect(parent.scrollTop).toBe(100);
  });
});
