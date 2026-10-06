import { describe, expect, it } from "vitest";
import { noteUpload } from "./note-upload";

describe("noteUpload", () => {
  it("saves the note as it is, in a markdown file named after the card", () => {
    const note = "# Plan\n\n- <b>one</b> & two\n";
    expect(noteUpload({ n: 7, title: "Fix token refresh" }, note)).toEqual({
      name: "Fix token refresh.md",
      content: note,
      mimeType: "text/markdown",
    });
  });

  it("makes nothing of a card with no note", () => {
    expect(noteUpload({ n: 7, title: "Fix" }, "")).toBeNull();
    expect(noteUpload({ n: 7, title: "Fix" }, " \n\t ")).toBeNull();
  });

  it("keeps a name that looks like a formula or markup as the plain name it is", () => {
    const upload = noteUpload({ n: 7, title: '=HYPERLINK("x") <b>&' }, "Note");
    expect(upload?.name).toBe('=HYPERLINK("x") <b>&.md');
  });

  it("takes the path marks out of the name, and keeps it on one line", () => {
    expect(noteUpload({ n: 7, title: "a/b\\c\nd" }, "Note")?.name).toBe("a-b-c d.md");
  });

  it("keeps the whole name within the limit, extension included", () => {
    const name = noteUpload({ n: 7, title: "x".repeat(500) }, "Note")?.name ?? "";
    expect(Array.from(name)).toHaveLength(200);
    expect(name.endsWith("….md")).toBe(true);
  });

  it("names a card with no title by its number", () => {
    expect(noteUpload({ n: 7, title: "   " }, "Note")?.name).toBe("Card #7.md");
  });
});
