import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { cardActions, moreItems } from "./card-actions";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const labels = (id: CardKey, mobile = false): string[] =>
  cardActions(cardOf(id), mobile).map((action) => action.label);

describe("cardActions", () => {
  beforeEach(() => resetStore());

  it("leads with Start card on a backlog card and hides Sleep", () => {
    expect(labels("api#45")).toEqual(["Start card", "Pin", "Fork"]);
    expect(cardActions(cardOf("api#45"), false)[0]?.primary).toBe(true);
  });

  it("offers Pause on a working card, with key hints on desktop only", () => {
    expect(labels("api#41")).toEqual(["Pause", "Sleep", "Pin", "Fork"]);
    const desktop = cardActions(cardOf("api#41"), false);
    expect(desktop.find((a) => a.label === "Sleep")?.kbd).toBe("S");
    expect(desktop.find((a) => a.label === "Pin")?.kbd).toBe("P");
    const phone = cardActions(cardOf("api#41"), true);
    expect(phone.every((a) => a.kbd === undefined)).toBe(true);
  });

  it("puts the pending plan approval first, and the session control after it", () => {
    const actions = cardActions(cardOf("api#43"), false);
    expect(actions[0]).toMatchObject({ label: "Approve plan", primary: true, kbd: "A" });
    expect(actions.some((a) => a.primary && a.label !== "Approve plan")).toBe(false);
  });

  it("puts the pending command approval first", () => {
    expect(cardActions(cardOf("api#44"), false)[0]).toMatchObject({ label: "Approve", kbd: "A" });
  });

  it("offers Resume session, disabled while the card wakes", () => {
    expect(labels("web#115")).toContain("Resume session");
    cardOf("web#115").waking = true;
    const waking = cardActions(cardOf("web#115"), false)[0];
    expect(waking).toMatchObject({ label: "Waking", disabled: true });
  });

  it("offers Resume card on a paused card", () => {
    cardOf("api#41").paused = true;
    expect(labels("api#41")[0]).toBe("Resume card");
  });

  it("offers Merge on a ready card, and no Pin on a done card", () => {
    expect(labels("api#36")).toContain("Merge");
    expect(labels("api#33")).toEqual(["Fork"]);
  });

  it("shows Unpin for a pinned card", () => {
    expect(labels("mobile#207")).toContain("Unpin");
  });

  it("queues a merge", () => {
    cardActions(cardOf("api#36"), false)
      .find((a) => a.label === "Merge")
      ?.run();
    expect(cardOf("api#36")).toMatchObject({ state: "merging", mergePct: 10 });
    expect(M.S.toasts.at(-1)?.msg).toBe("Added to merge queue");
  });
});

describe("moreItems", () => {
  beforeEach(() => resetStore());

  it("lists a move to every other column, then the fixed items", () => {
    const items = moreItems(cardOf("api#41"), () => undefined);
    const moves = items.filter((item) => item.label.startsWith("Move to "));
    expect(moves).toHaveLength(M.COLUMNS.length - 1);
    expect(moves.map((item) => item.label)).not.toContain("Move to working");
    expect(items.slice(-4).map((item) => item.label)).toEqual([
      "Restore a checkpoint",
      "Simulate CI failure",
      "Copy branch name",
      "Delete card",
    ]);
    expect(items.at(-1)?.danger).toBe(true);
  });

  it("closes the menu before it runs an item", () => {
    const close = vi.fn();
    const items = moreItems(cardOf("api#41"), close);
    items.find((item) => item.label === "Restore a checkpoint")?.run();
    expect(close).toHaveBeenCalledOnce();
    expect(M.S.tab).toBe("activity");
  });

  it("copies the branch name and says so", () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    moreItems(cardOf("api#41"), () => undefined)
      .find((item) => item.label === "Copy branch name")
      ?.run();
    expect(writeText).toHaveBeenCalledWith("marshal/41-fix-token-refresh");
    expect(M.S.toasts.at(-1)?.msg).toBe("Branch name copied");
    vi.unstubAllGlobals();
  });

  it("asks before it deletes", () => {
    moreItems(cardOf("api#41"), () => undefined)
      .find((item) => item.label === "Delete card")
      ?.run();
    expect(M.S.dialog?.title).toBe("Delete card");
  });
});
