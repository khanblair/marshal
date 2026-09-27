import type { CheckpointList } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "../testing/golden";
import { toCheckpointRow, toCheckpointRows } from "./checkpoints";

/* The list the real daemon answers with, written by its own Go tests, so the two sides cannot drift. */
const list = golden<CheckpointList>("checkpoint-list");

/** The golden list as the store holds it: the same order, the time in ms, nothing added. */
const expected = list.checkpoints.map((cp) => ({
  id: cp.id,
  cardId: cp.cardId,
  sha: cp.sha,
  label: cp.label,
  at: Date.parse(cp.createdAt),
}));

describe("mapping a card's restore points", () => {
  it("keeps the daemon's order, newest first, and every field it sent", () => {
    expect(toCheckpointRows(list)).toEqual(expected);
    expect(toCheckpointRows(list)).toHaveLength(3);
  });

  it("turns each time into milliseconds, which is what the screens count from", () => {
    const [first] = toCheckpointRows(list);
    expect(first?.at).toBe(Date.parse("2026-09-27T09:30:00.000Z"));
    expect(typeof first?.at).toBe("number");
  });

  it("keeps the full commit, leaving the short form to the screen that draws it", () => {
    const [first] = toCheckpointRows(list);
    expect(first?.sha).toBe("9f2c1b7a4e6d85031234567890abcdef01234567");
  });

  it("keeps an empty label empty, rather than inventing one the daemon did not send", () => {
    const rows = toCheckpointRows(list);
    expect(rows.at(-1)?.label).toBe("");
  });

  it("maps one checkpoint on its own, and an empty list to no rows at all", () => {
    const one = list.checkpoints[1];
    if (!one) throw new Error("the golden list has fewer than two checkpoints");
    expect(toCheckpointRow(one)).toEqual(expected[1]);
    expect(
      toCheckpointRows({ cardId: list.cardId, checkpoints: [], serverTime: list.serverTime }),
    ).toEqual([]);
  });
});
