import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { cardOf, resetStore } from "~/views/card/test-helpers";
import { noteProblem, noteState } from "./note-state";

const sections = vi.hoisted(() => ({ s14Daemon: false }));

// One flag says whether card notes are the daemon's, so both paths are read here.
vi.mock("~/data/sections", async (original) => {
  const real = await original<typeof import("~/data/sections")>();
  return {
    ...real,
    isDaemon: (id: string, table?: never) =>
      id === "S14" ? sections.s14Daemon : real.isDaemon(id as never, table),
  };
});

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const note = (body: string, updatedAt: number | null): void => {
  M.S.notes = { ...M.S.notes, "api#41": body };
  M.S.noteInfo = {
    ...M.S.noteInfo,
    "api#41": { path: "api/cards/41-x.md", author: "person", updatedAt },
  };
};

describe("the state of a card's note", () => {
  beforeEach(() => {
    resetStore();
    delete M.S.noteInfo;
    sections.s14Daemon = false;
  });

  it("is saved for the mock's own note, and none once it is emptied", () => {
    expect(noteState(cardOf("api#41"))).toBe("saved");
    expect(noteProblem(cardOf("api#41"))).toBeNull();
    M.S.notes = { "api#41": "  \n" };
    expect(noteState(cardOf("api#41"))).toBe("none");
    expect(noteProblem(cardOf("api#41"))).toBe("This card has no note yet.");
  });

  it("is loading on the daemon until the note has been read", () => {
    sections.s14Daemon = true;
    expect(noteState(cardOf("api#41"))).toBe("loading");
    expect(noteProblem(cardOf("api#41"))).toBe("The note is still loading. Try again in a moment.");
  });

  it("is none on the daemon for a note nothing was saved for, and saved once something was", () => {
    sections.s14Daemon = true;
    note("A starter note", null);
    expect(noteState(cardOf("api#41"))).toBe("none");
    note("My note", Date.now());
    expect(noteState(cardOf("api#41"))).toBe("saved");
    note("", Date.now());
    expect(noteState(cardOf("api#41"))).toBe("none");
  });
});
