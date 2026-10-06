import { describe, expect, it } from "vitest";
import { clip, dayOf, htmlText, minuteOf, oneLine, titleOf } from "./export-text";

describe("export text", () => {
  it("puts text on one line and cuts it without splitting a character", () => {
    expect(oneLine("  a\n\tb \u0007 c\r\n")).toBe("a b c");
    expect(clip("abc", 3)).toBe("abc");
    expect(clip("abcd", 3)).toBe("ab…");
    expect(Array.from(clip("😀😀😀😀", 3))).toEqual(["😀", "😀", "…"]);
  });

  it("names a card by its title, else by its number", () => {
    expect(titleOf({ n: 3, title: " Fix\nit " })).toBe("Fix it");
    expect(titleOf({ n: 3, title: "" })).toBe("Card #3");
  });

  it("writes a time as a day or a minute in UTC, and nothing for a time that is not there", () => {
    const at = Date.UTC(2026, 0, 2, 3, 4, 59);
    expect(dayOf(at)).toBe("2026-01-02");
    expect(minuteOf(at)).toBe("2026-01-02 03:04 UTC");
    for (const bad of [null, undefined, Number.NaN, Number.POSITIVE_INFINITY]) {
      expect(dayOf(bad)).toBe("");
      expect(minuteOf(bad)).toBe("");
    }
  });

  it("escapes text for a document and keeps its line breaks", () => {
    expect(htmlText(` <a href="x">&'</a>\nnext `)).toBe(
      "&lt;a href=&quot;x&quot;&gt;&amp;&#39;&lt;/a&gt;<br>next",
    );
  });
});
