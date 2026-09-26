import { describe, expect, it } from "vitest";
import type { DiffFile } from "~/mock";
import { allOpen, estimateFileHeight, hunkViews, openPaths, toggled } from "./diff-model";

const file = (path: string, large = false): DiffFile => ({
  path,
  add: 3,
  del: 1,
  large,
  hunks: [
    {
      h: "@@ -1,3 +1,4 @@",
      lines: [
        [" ", 1, "keep"],
        ["-", 2, "old"],
        ["+", 2, "new"],
      ],
    },
  ],
});

describe("openPaths", () => {
  const diff = [file("a.go", true), file("b.go"), file("c.go"), file("d.go")];

  it("opens the first two files that are not large until the user chooses", () => {
    expect([...openPaths(diff, null)]).toEqual(["b.go", "c.go"]);
  });

  it("uses the user's choice, even when it is empty", () => {
    expect([...openPaths(diff, ["d.go"])]).toEqual(["d.go"]);
    expect(openPaths(diff, []).size).toBe(0);
  });
});

describe("toggled and allOpen", () => {
  it("adds a closed path and removes an open one", () => {
    expect(toggled(new Set(["a"]), "b").sort()).toEqual(["a", "b"]);
    expect(toggled(new Set(["a", "b"]), "a")).toEqual(["b"]);
  });

  it("lists every path", () => {
    expect(allOpen([file("a.go"), file("b.go")])).toEqual(["a.go", "b.go"]);
  });
});

describe("hunkViews", () => {
  it("colors each line by its sign and uses the minus sign character", () => {
    const [hunk] = hunkViews(file("a.go"));
    expect(hunk?.header).toBe("@@ -1,3 +1,4 @@");
    expect(hunk?.lines.map((line) => line.sign)).toEqual(["", "−", "+"]);
    expect(hunk?.lines.map((line) => line.bgClass)).toEqual([
      "bg-transparent",
      "bg-diff-removed-bg",
      "bg-diff-added-bg",
    ]);
    expect(hunk?.lines[2]).toMatchObject({
      n: 2,
      text: "new",
      gutterClass: "bg-diff-added-line",
      textClass: "text-diff-added-text",
    });
  });

  it("draws exactly the lines a file has, large or not", () => {
    const large = file("go.sum", true);
    expect(hunkViews(large)[0]?.header).toBe("@@ -1,3 +1,4 @@");
    expect(hunkViews(large)[0]?.lines).toHaveLength(3);
    expect(hunkViews({ ...large, hunks: [] })).toEqual([]);
  });
});

describe("estimateFileHeight", () => {
  const hunks = file("a.go").hunks;

  it("is the height of the header for a closed file, whatever it holds", () => {
    expect(estimateFileHeight(false, [])).toBe(38);
    expect(estimateFileHeight(false, hunks)).toBe(38);
  });

  it("adds a line for the hunk's header and one for each of its lines when the file is open", () => {
    // One hunk of three lines is four lines of 20 px under the 38 px header.
    expect(estimateFileHeight(true, hunks)).toBe(38 + 4 * 20);
    expect(estimateFileHeight(true, [...hunks, ...hunks])).toBe(38 + 8 * 20);
  });

  it("gives an open file that has no lines yet room for its notice or its first hunk", () => {
    expect(estimateFileHeight(true, [])).toBe(38 + 3 * 20);
  });
});
