import { describe, expect, it } from "vitest";
import type { TrelloChangeView } from "~/data/mappers/trello";
import {
  applyRequest,
  initialSelection,
  pickedCount,
  reconcileSelection,
  setPicked,
  togglePicked,
} from "./trello-apply";

function change(id: string, conflict = false): TrelloChangeView {
  return {
    id,
    cardId: "card-1",
    cardTitle: "A card",
    kind: conflict ? "moveDone" : "addComment",
    summary: id,
    detail: "",
    by: "agent",
    byLabel: "by the agent",
    conflict,
    conflictNote: conflict ? "Trello moved this card." : "",
  };
}

const calm = change("calm");
const other = change("other");
const clash = change("clash", true);

describe("the starting choice", () => {
  it("ticks every change except one in conflict", () => {
    const selection = initialSelection([calm, clash, other]);
    expect([...selection.picked]).toEqual(["calm", "other"]);
    expect(selection.overridden.size).toBe(0);
  });
});

describe("ticking a change", () => {
  it("ticks a conflict together with its override, and unticks both", () => {
    const start = initialSelection([calm, clash]);
    const on = togglePicked(start, clash);
    expect(on.picked.has("clash")).toBe(true);
    expect(on.overridden.has("clash")).toBe(true);
    const off = togglePicked(on, clash);
    expect(off.picked.has("clash")).toBe(false);
    expect(off.overridden.has("clash")).toBe(false);
  });

  it("never overrides a change that is not in conflict", () => {
    const selection = setPicked(initialSelection([calm]), calm, true);
    expect(selection.overridden.size).toBe(0);
    expect(togglePicked(selection, calm).picked.has("calm")).toBe(false);
  });
});

describe("the request", () => {
  it("lists the picked changes in list order, with overrides only inside them", () => {
    const changes = [calm, clash, other];
    const selection = setPicked(initialSelection(changes), clash, true);
    expect(applyRequest(selection, changes)).toEqual({
      changeIds: ["calm", "clash", "other"],
      overrideIds: ["clash"],
    });
    expect(pickedCount(selection, changes)).toBe(3);
  });

  it("leaves a conflict out of both lists when it was not chosen", () => {
    const changes = [calm, clash];
    expect(applyRequest(initialSelection(changes), changes)).toEqual({
      changeIds: ["calm"],
      overrideIds: [],
    });
  });
});

describe("a list that changes while the dialog is open", () => {
  it("keeps the choice of a change that stays, and drops the ids that left", () => {
    const first = togglePicked(initialSelection([calm, other]), other);
    const next = reconcileSelection(first, [other, calm]);
    expect(next.picked.has("calm")).toBe(true);
    expect(next.picked.has("other")).toBe(false);
    const fewer = reconcileSelection(next, [calm]);
    expect([...fewer.known]).toEqual(["calm"]);
    expect(applyRequest(fewer, [calm]).changeIds).toEqual(["calm"]);
  });

  it("gives a new change its default, ticked, or off when it is in conflict", () => {
    const start = initialSelection([calm]);
    const next = reconcileSelection(start, [calm, other, clash]);
    expect(next.picked.has("other")).toBe(true);
    expect(next.picked.has("clash")).toBe(false);
  });

  it("drops the override of a conflict that left, and unticks a change that turned into one", () => {
    const picked = setPicked(initialSelection([calm, clash]), clash, true);
    expect(reconcileSelection(picked, [calm]).overridden.size).toBe(0);
    const turned = { ...calm, conflict: true, conflictNote: "Trello changed this." };
    const next = reconcileSelection(initialSelection([calm]), [turned]);
    expect(next.picked.has("calm")).toBe(false);
  });
});
