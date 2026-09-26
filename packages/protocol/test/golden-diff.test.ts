import { describe, expect, it } from "vitest";
import type { CardDiff, FileHunks } from "../src";
import { golden } from "./golden";

// A card's diff (docs/backend-checklist.md B2.9, N15): the changed files with their counts, and one
// file's hunks when the screen opens it. The list carries a file of every status, a binary file,
// and a large file that the screen keeps collapsed; the hunks carry a line of every kind.
describe("the card diff golden files", () => {
  it("has a card's changed files with their counts and no hunks", () => {
    const sample: CardDiff = {
      cardId: "01M3C107JB041061050R3GG28A",
      base: "main",
      branch: "marshal/api-41-refresh-the-token",
      files: [
        {
          path: "assets/logo.png",
          oldPath: "",
          status: "modified",
          additions: 0,
          deletions: 0,
          binary: true,
          large: false,
        },
        {
          path: "go.sum",
          oldPath: "",
          status: "added",
          additions: 1240,
          deletions: 0,
          binary: false,
          large: true,
        },
        {
          path: "internal/auth/legacy.go",
          oldPath: "",
          status: "deleted",
          additions: 0,
          deletions: 41,
          binary: false,
          large: false,
        },
        {
          path: "internal/auth/middleware.go",
          oldPath: "",
          status: "modified",
          additions: 19,
          deletions: 3,
          binary: false,
          large: false,
        },
        {
          path: "internal/auth/refresh.go",
          oldPath: "",
          status: "modified",
          additions: 38,
          deletions: 9,
          binary: false,
          large: false,
        },
        {
          path: "internal/auth/session.go",
          oldPath: "internal/auth/token.go",
          status: "renamed",
          additions: 2,
          deletions: 2,
          binary: false,
          large: false,
        },
      ],
      fileCount: 1203,
      additions: 1299,
      deletions: 55,
      truncated: true,
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("card-diff")).toEqual(sample);
    // The totals describe the whole diff, not only the files that came back, and the file the
    // screen keeps collapsed says so.
    expect(sample.fileCount).toBeGreaterThan(sample.files.length);
    expect(sample.truncated).toBe(true);
    expect(sample.files[1]?.large).toBe(true);
  });

  it("has one file's hunks, with the side each line is on", () => {
    const sample: FileHunks = {
      path: "internal/auth/refresh.go",
      status: "modified",
      hunks: [
        {
          header: "@@ -41,5 +41,8 @@ type Client struct",
          lines: [
            { kind: "context", oldLine: 41, newLine: 41, text: "type Client struct {" },
            { kind: "removed", oldLine: 42, newLine: 0, text: "\thttp *http.Client" },
            { kind: "added", oldLine: 0, newLine: 42, text: "\thttp     *http.Client" },
            { kind: "added", oldLine: 0, newLine: 43, text: "\tgroup    singleflight.Group" },
            { kind: "context", oldLine: 43, newLine: 44, text: "}" },
          ],
        },
      ],
      truncated: false,
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("file-hunks")).toEqual(sample);
    // A removed line is not on the new side and an added line is not on the old one, so the view
    // draws one number without guessing which column it belongs in.
    expect(sample.hunks[0]?.lines[1]?.newLine).toBe(0);
    expect(sample.hunks[0]?.lines[2]?.oldLine).toBe(0);
  });

  it("has an empty diff for a card that never started, as a list and never null", () => {
    const sample: CardDiff = {
      cardId: "01M3C107JB041061050R3GG28B",
      base: "main",
      branch: "",
      files: [],
      fileCount: 0,
      additions: 0,
      deletions: 0,
      truncated: false,
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("card-diff-empty")).toEqual(sample);
  });
});
