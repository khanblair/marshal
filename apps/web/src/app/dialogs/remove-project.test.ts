import { afterEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { removeModel } from "./remove-project";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const seedCards = JSON.parse(JSON.stringify(M.S.cards));
afterEach(() => {
  M.S.cards = structuredClone(seedCards);
});

describe("removeModel", () => {
  it("is null for a project that does not exist", () => {
    expect(removeModel(M, "nope")).toBeNull();
  });

  it("counts running agents, worktrees, cards, and chats", () => {
    const model = removeModel(M, "web");
    expect(model?.name).toBe("web-dashboard");
    expect(model?.memoryPath).toBe("vault/projects/web-dashboard/");
    const cards = M.cardsOf("web");
    const unmerged = cards.filter((c) => c.branch && c.state !== "done").length;
    const awake = cards.filter((c) => M.isAwake(c)).length;
    const chats = (M.S.chats.web || []).length;
    expect(model?.unmerged).toBe(unmerged);
    expect(model?.effects).toEqual([
      { icon: "square", text: `Running sessions stop. ${awake} agents are awake now.` },
      { icon: "folder-minus", text: `Worktrees are cleaned up. ${unmerged} worktrees.` },
      {
        icon: "trash-2",
        text: `Cards and chats are removed from Marshal. ${cards.length} cards and ${chats} chats.`,
      },
    ]);
  });

  it("says card in the singular for one unmerged card, and has none to report when all are merged", () => {
    const [first, ...rest] = M.S.cards.filter(
      (c) => c.p === "web" && c.branch && c.state !== "done",
    );
    for (const c of rest) c.branch = null;
    expect(removeModel(M, "web")).toMatchObject({
      unmerged: 1,
      unmergedLabel: "1 card has unmerged work",
    });
    if (first) first.branch = null;
    expect(removeModel(M, "web")?.unmerged).toBe(0);
  });

  it("uses the plural label for several unmerged cards", () => {
    expect(removeModel(M, "api")?.unmergedLabel).toMatch(/^\d+ cards have unmerged work$/);
  });
});
