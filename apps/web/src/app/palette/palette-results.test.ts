import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
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
    expect(needs.map((x) => x.label.split(" ")[0])).toEqual(["#43", "#119", "#210", "#44"]);
    expect(list.some((x) => x.group === "Cards")).toBe(false);
  });

  it("treats a blank query as no query", () => {
    expect(paletteResults(M, "   ").map((x) => x.label)).toEqual(
      paletteResults(M, "").map((x) => x.label),
    );
  });

  it("keeps commands that contain every word, in any order, ignoring case", () => {
    const labels = paletteResults(M, "LIMITING rate").map((x) => x.label);
    expect(labels).toEqual(["#43 Add rate limiting per API key"]);
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
