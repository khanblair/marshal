import { beforeEach, describe, expect, it, vi } from "vitest";
import { cardTabs, neighborTab, TAB_KEYS } from "./card-tabs";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

describe("cardTabs", () => {
  beforeEach(() => resetStore());

  it("lists the seven tabs in order", () => {
    expect(cardTabs(cardOf("api#41")).map((tab) => tab.label)).toEqual([
      "Chat",
      "Comments",
      "Activity",
      "Diff",
      "Checklists",
      "Preview",
      "Notes",
    ]);
  });

  it("counts comments, activity, diff files, and done over total for checklists and checks", () => {
    const tabs = Object.fromEntries(cardTabs(cardOf("api#41")).map((tab) => [tab.key, tab.count]));
    expect(tabs.comments).toBe(cardOf("api#41").comments.length);
    expect(tabs.diff).toBe(4);
    expect(tabs.chat).toBeUndefined();
    expect(String(tabs.checks)).toMatch(/^\d+\/\d+$/);
  });

  it("leaves out the count when there is nothing to count", () => {
    const tabs = Object.fromEntries(cardTabs(cardOf("api#45")).map((tab) => [tab.key, tab.count]));
    expect(tabs.comments).toBeUndefined();
    expect(tabs.diff).toBeUndefined();
  });

  it("marks the checklist tab failed when an acceptance check failed", () => {
    expect(cardTabs(cardOf("web#119")).find((tab) => tab.key === "checks")?.failed).toBe(true);
    expect(cardTabs(cardOf("api#41")).find((tab) => tab.key === "checks")?.failed).toBe(false);
  });
});

describe("neighborTab", () => {
  it("moves right and left, wrapping around the ends", () => {
    expect(neighborTab("chat", "ArrowRight")).toBe("comments");
    expect(neighborTab("notes", "ArrowRight")).toBe("chat");
    expect(neighborTab("chat", "ArrowLeft")).toBe("notes");
    expect(TAB_KEYS).toHaveLength(7);
  });
});
