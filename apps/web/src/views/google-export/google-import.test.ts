import type { GoogleLinkContent } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { linkService, noteWithImport } from "./google-import";

const content = (fields: Partial<GoogleLinkContent> = {}): GoogleLinkContent => ({
  kind: "doc",
  id: "abc",
  title: "Launch plan",
  url: "https://docs.google.com/document/d/abc/edit",
  markdown: "# Launch plan\n\n- Write the notes",
  truncated: false,
  ...fields,
});

describe("linkService", () => {
  it.each([
    ["https://docs.google.com/document/d/abc/edit", "gdocs"],
    ["https://docs.google.com/document/u/0/d/abc/edit", "gdocs"],
    ["  https://docs.google.com/spreadsheets/d/s1/edit#gid=0  ", "gsheets"],
    ["https://docs.google.com/u/1/presentation/d/p1/edit", "gslides"],
  ])("names the connection %s needs", (link, service) => {
    expect(linkService(link)).toBe(service);
  });

  it.each([
    "",
    "not a link",
    "https://example.com/document/d/abc",
    "https://docs.google.com/forms/d/abc/edit",
    "https://docs.google.com.evil.test/document/d/abc",
    "https://docs.google.com/",
  ])("names nothing for %s, and leaves the daemon to say so", (link) => {
    expect(linkService(link)).toBeNull();
  });
});

describe("noteWithImport", () => {
  it("adds a headed block with the markdown and the link after what the note says", () => {
    expect(noteWithImport("# Card\n\nMy words\n\n", content())).toBe(
      [
        "# Card",
        "",
        "My words",
        "",
        "## From Google Doc: Launch plan",
        "",
        "# Launch plan",
        "",
        "- Write the notes",
        "",
        "Source: https://docs.google.com/document/d/abc/edit",
        "",
      ].join("\n"),
    );
  });

  it("starts the note with the block when the note is empty", () => {
    expect(noteWithImport("  \n", content({ kind: "sheet", title: "Budget" }))).toMatch(
      /^## From Google Sheet: Budget\n\n# Launch plan/,
    );
  });

  it("names a presentation Slides, and keeps a title with line breaks on one line", () => {
    const block = noteWithImport("x", content({ kind: "slides", title: "Q4\nreview\r\n" }));
    expect(block).toContain("## From Google Slides: Q4 review\n");
  });

  it("leaves the link out when the daemon gave none", () => {
    expect(noteWithImport("", content({ url: "" }))).not.toContain("Source:");
  });
});
