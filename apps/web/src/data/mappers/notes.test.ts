import type { Note } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "~/data/testing/golden";
import { toNoteInfo } from "./notes";

// Section S14: the Notes tab. The mapper is tested from the two goldens the daemon's own contract
// test writes (`daemon/testdata/golden/note.json`, `note-unsaved.json`), so the two sides cannot
// drift: the same shape is what GET and PUT /v1/cards/{id}/note both answer with.

describe("the note mapper", () => {
  it("keeps a saved note's path, author, and time, in ms", () => {
    const note = golden<Note>("note");
    const info = toNoteInfo(note);
    expect(info.path).toBe("small-repo/cards/7-add-a-health-check.md");
    expect(info.author).toBe("person");
    expect(info.updatedAt).toBe(Date.parse("2026-09-27T09:30:00.000Z"));
  });

  it("reads a null save time as null, not a date in the year 1", () => {
    const note = golden<Note>("note-unsaved");
    const info = toNoteInfo(note);
    expect(info.updatedAt).toBeNull();
    expect(info.path).toBe("small-repo/cards/9-write-the-runbook.md");
  });
});
