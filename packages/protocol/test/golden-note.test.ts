import { describe, expect, it } from "vitest";
import {
  type Note,
  NoteAuthorAgent,
  NoteAuthorPerson,
  NoteAuthorValues,
  type SaveNoteRequest,
} from "../src";
import { golden } from "./golden";

// A card's note (docs/backend-checklist.md B7.4 and N11, build-plan task 7.12). Each sample is
// checked against the generated type by the compiler, so a field that changes in Go stops this
// compiling until the sample matches the daemon's own file again.
describe("the note golden files", () => {
  it("has a saved note with its text, its vault path, and the daemon's time", () => {
    const sample: Note = {
      cardId: "card-7f2a",
      projectId: "small-repo",
      path: "small-repo/cards/7-add-a-health-check.md",
      body: "# Add a health check\n\nGoal: add a health check.\n\nThe probe goes beside the server.\n\nLinks\n[[small-repo]]\n",
      author: "person",
      updatedAt: "2026-09-27T09:30:00.000Z",
    };
    expect(golden("note")).toEqual(sample);
    // The path is relative to the vault root: the vault lives wherever the person put it, so an
    // absolute path from the daemon's machine would be meaningless to a phone.
    expect(sample.path.startsWith("/")).toBe(false);
  });

  it("has a note nothing has been saved for, with a null time and no file yet", () => {
    const sample: Note = {
      cardId: "card-9c14",
      projectId: "small-repo",
      path: "small-repo/cards/9-write-the-runbook.md",
      body: "# Write the runbook\n\nGoal: write the runbook.\n\nLinks\n[[small-repo]]\n",
      author: "person",
      updatedAt: null,
    };
    expect(golden("note-unsaved")).toEqual(sample);
    // `updatedAt` is required and nullable, not optional: the daemon always sends it, as null when
    // nothing has been saved, which is the whole of what "no note" means on the wire.
    expect(sample.updatedAt).toBeNull();
  });

  it("has the two authors the rest of the schema uses", () => {
    expect(NoteAuthorValues).toEqual(["person", "agent"]);
    expect(NoteAuthorPerson).toBe("person");
    expect(NoteAuthorAgent).toBe("agent");
  });

  it("sends a note back whole, so its own punctuation survives the round trip", () => {
    // A save is the whole note and not a patch: the tab's editor replaces the file.
    const body = (golden("note") as Note).body;
    const request: SaveNoteRequest = { body };
    expect(request.body).toBe(body);
  });
});
