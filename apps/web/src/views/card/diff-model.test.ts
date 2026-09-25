import { describe, expect, it } from "vitest";
import type { DiffFile } from "~/mock";
import { allOpen, hunkViews, openPaths, toggled } from "./diff-model";

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
    const [hunk] = hunkViews(file("a.go"), false);
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

  it("shows the sample lines only after a large file is loaded", () => {
    const large = file("go.sum", true);
    expect(hunkViews(large, false)[0]?.header).toBe("@@ -1,3 +1,4 @@");
    const loaded = hunkViews(large, true);
    expect(loaded[0]?.header).toBe("@@ -0,0 +1,1240 @@");
    expect(loaded[0]?.lines).toHaveLength(3);
  });
});
