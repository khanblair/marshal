import { describe, expect, it } from "vitest";
import { renderMarkdown } from "./render";

const html = (source: string) => {
  const box = document.createElement("div");
  box.innerHTML = renderMarkdown(source);
  return box;
};

describe("renderMarkdown", () => {
  it("renders bold, italic, inline code, and a line break from a single newline", () => {
    const box = html("**Bold** and *soft* with `code`\nnext line");
    expect(box.querySelector("strong")?.textContent).toBe("Bold");
    expect(box.querySelector("em")?.textContent).toBe("soft");
    expect(box.querySelector("code")?.textContent).toBe("code");
    expect(box.querySelector("br")).not.toBeNull();
  });

  it("renders headings, lists, fenced code, quotes, and tables", () => {
    const box = html(
      "# Title\n\n- one\n- two\n\n1. a\n2. b\n\n```ts\nconst x = 1;\n```\n\n> quoted\n\n| a | b |\n|---|---|\n| 1 | 2 |",
    );
    expect(box.querySelector("h1")?.textContent).toBe("Title");
    expect(box.querySelectorAll("ul li")).toHaveLength(2);
    expect(box.querySelectorAll("ol li")).toHaveLength(2);
    expect(box.querySelector("pre code")?.textContent).toContain("const x = 1;");
    expect(box.querySelector("blockquote")?.textContent).toContain("quoted");
    expect(box.querySelectorAll("td")).toHaveLength(2);
  });

  it("shows raw HTML as text and never runs it", () => {
    const box = html('<script>alert(1)</script><img src=x onerror="alert(1)"><b>hi</b>');
    expect(box.querySelector("script")).toBeNull();
    expect(box.querySelector("img")).toBeNull();
    expect(box.querySelector("b")).toBeNull();
    expect(box.textContent).toContain("<script>");
  });

  it("drops script, javascript, and data links but keeps web and mail links", () => {
    const box = html(
      "[a](javascript:alert(1)) [b](data:text/html,x) [c](https://example.com) [d](mailto:a@b.co)",
    );
    // An unsafe link stays as text with no address, so it goes nowhere when clicked.
    const hrefs = [...box.querySelectorAll("a")].map((a) => a.getAttribute("href"));
    expect(hrefs).toEqual([null, null, "https://example.com", "mailto:a@b.co"]);
    expect(box.querySelector('a[href^="https"]')?.getAttribute("rel")).toBe("noopener noreferrer");
    expect(box.querySelector('a[href^="https"]')?.getAttribute("target")).toBe("_blank");
  });

  it("turns an image into a plain link so nothing loads on its own", () => {
    const box = html("![chart](https://example.com/c.png)");
    expect(box.querySelector("img")).toBeNull();
    expect(box.querySelector("a")?.textContent).toBe("chart");
  });

  it("makes task-list boxes read-only", () => {
    const box = html("- [x] done\n- [ ] todo");
    const boxes = [...box.querySelectorAll("input")];
    expect(boxes).toHaveLength(2);
    expect(boxes.every((b) => b.disabled && b.type === "checkbox")).toBe(true);
  });

  it("copes with half-written markdown while a reply streams", () => {
    expect(() => renderMarkdown("**bold with no end\n```ts\nopen fence")).not.toThrow();
    expect(renderMarkdown("")).toBe("");
  });
});
