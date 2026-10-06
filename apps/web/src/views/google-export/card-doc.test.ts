import { describe, expect, it } from "vitest";
import { type CardDocInput, cardDoc } from "./card-doc";

const NAMES: Record<string, string> = { ada: "Ada Okafor", blair: "Blair" };
const WHEN = Date.UTC(2026, 9, 6, 14, 5, 59);

function input(overrides: Partial<CardDocInput["card"]> = {}, note = ""): CardDocInput {
  return {
    card: {
      n: 41,
      title: "Fix token refresh",
      doing: "Reading the diff",
      checklists: [
        {
          id: "cl1",
          title: "Before merge",
          hideDone: false,
          items: [
            { id: "i1", text: "Add a test", done: true, by: "ada" },
            { id: "i2", text: "Update the docs", done: false, by: null },
          ],
        },
      ],
      comments: [
        { id: "c1", author: "ada", text: "Looks good\nShip it", ts: WHEN, att: [], read: true },
      ],
      ...overrides,
    },
    state: "In review",
    project: "api-gateway",
    note,
    nameOf: (id) => NAMES[id] ?? id,
  };
}

describe("cardDoc", () => {
  it("names the document after the card and writes what the card holds", () => {
    const doc = cardDoc(input({}, "# Plan\n\n- one"));
    expect(doc.title).toBe("Fix token refresh");
    expect(doc.text).toBeUndefined();
    expect(doc.html).toContain("<h1>Fix token refresh</h1>");
    expect(doc.html).toContain("<p>Reading the diff</p>");
    expect(doc.html).toContain("<strong>State:</strong> In review");
    expect(doc.html).toContain("<strong>Project:</strong> api-gateway");
    expect(doc.html).toContain("<h3>Before merge</h3>");
    expect(doc.html).toContain("<li>☑ Add a test</li>");
    expect(doc.html).toContain("<li>☐ Update the docs</li>");
    expect(doc.html).toContain("<h2>Note</h2>");
    expect(doc.html).toContain("<h1>Plan</h1>");
    expect(doc.html).toContain("<li>one</li>");
  });

  it("gives each comment its author and its time, and keeps its line breaks", () => {
    expect(cardDoc(input()).html).toContain(
      "<p><strong>Ada Okafor</strong> · 2026-10-06 14:05 UTC<br>Looks good<br>Ship it</p>",
    );
  });

  it("falls back to the author's id when it is nobody the app knows, and leaves a bad time out", () => {
    const html = cardDoc(
      input({
        comments: [{ id: "c", author: "ghost", text: "Hi", ts: Number.NaN, att: [], read: true }],
      }),
    ).html;
    expect(html).toContain("<p><strong>ghost</strong><br>Hi</p>");
  });

  it("escapes every piece of card text, so none of it is markup", () => {
    const hostile = `<script>alert("x")</script> & 'q'`;
    const escaped = "&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt; &amp; &#39;q&#39;";
    const doc = cardDoc({
      ...input({
        title: hostile,
        doing: hostile,
        checklists: [
          {
            id: "cl",
            title: hostile,
            hideDone: false,
            items: [{ id: "i", text: hostile, done: false, by: null }],
          },
        ],
        comments: [{ id: "c", author: "evil", text: hostile, ts: WHEN, att: [], read: true }],
      }),
      state: hostile,
      project: hostile,
      nameOf: () => hostile,
    });
    expect(doc.html).not.toContain("<script");
    expect(doc.html?.split(escaped).length).toBe(9);
    expect(doc.title).toBe(hostile);
  });

  it("keeps a title that looks like a formula as plain text", () => {
    const doc = cardDoc(input({ title: '=HYPERLINK("http://evil","click")' }));
    expect(doc.html).toContain("<h1>=HYPERLINK(&quot;http://evil&quot;,&quot;click&quot;)</h1>");
  });

  it("sends the note through the markdown cleaner, so a script in it is dropped", () => {
    const html = cardDoc(
      input({}, "Hello <script>alert(1)</script> **bold**\n\n<img src=x onerror=alert(1)>"),
    ).html;
    expect(html).not.toContain("<script");
    expect(html).not.toContain("<img");
    expect(html).toContain("<strong>bold</strong>");
  });

  it("leaves out the sections a card has nothing for", () => {
    const doc = cardDoc(input({ doing: "  ", checklists: [], comments: [] }));
    expect(doc.html).toBe(
      [
        "<h1>Fix token refresh</h1>",
        "<p><strong>State:</strong> In review<br><strong>Project:</strong> api-gateway</p>",
      ].join("\n"),
    );
  });

  it("says so for a checklist with no lines, and names one that has no title", () => {
    const html = cardDoc(
      input({
        checklists: [{ id: "cl", title: " ", hideDone: false, items: [] }],
      }),
    ).html;
    expect(html).toContain("<h3>Checklist</h3><p>No items.</p>");
  });

  it("keeps a title within the limit and on one line, and names a card with no title", () => {
    const long = cardDoc(input({ title: `a\nb ${"x".repeat(300)}` }));
    expect(Array.from(long.title)).toHaveLength(200);
    expect(long.title.startsWith("a b xxx")).toBe(true);
    expect(cardDoc(input({ title: "  " })).title).toBe("Card #41");
  });
});
